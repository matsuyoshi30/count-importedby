package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/semgroup"
	"golang.org/x/net/html"
	"golang.org/x/tools/go/packages"
)

var stdlibs []string

const pkgURL = "https://pkg.go.dev/"

func init() {
	pkgs, err := packages.Load(nil, "std")
	if err != nil {
		panic(err)
	}
	stdlibs = make([]string, len(pkgs))
	for i, pkg := range pkgs {
		stdlibs[i] = pkg.ID
	}
}

type Result struct {
	LibPath string `json:"libPath"`
	Num     int    `json:"num"`
}

func main() {
	resp, err := http.Get(pkgURL)
	if err != nil {
		fmt.Println(err)
		return
	} else if resp.StatusCode != http.StatusOK {
		fmt.Println("pkg.go.dev returns not 200")
		return
	}

	results := make([]Result, 0)

	sg := semgroup.NewGroup(context.Background(), 1)
	for _, stdlib := range removeInternalPkg(stdlibs) {
		stdlib := stdlib
		sg.Go(func() error {
			log.Printf("[INFO] start request about package %s\n", stdlib)
			resp, err := http.Get(pkgURL + stdlib)
			log.Printf("[INFO] finish request about package %s\n", stdlib)
			if err != nil {
				return err
			} else if resp.StatusCode != http.StatusOK {
				return nil
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("[ERR ] package %s: %q\n", stdlib, err)
				return err
			}

			doc, err := html.Parse(bytes.NewReader(body))
			if err != nil {
				log.Printf("[ERR ] package %s: %q\n", stdlib, err)
				return err
			}

			ret := extractImportedBy(doc)
			if ret == "" {
				err := fmt.Errorf("failed to extract imported-by value about %s", stdlib)
				log.Printf("[ERR ] package %s: %q\n", stdlib, err)
				return err
			}

			importedBy, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(ret), ",", ""))
			if err != nil {
				log.Printf("[ERR ] package %s: %q\n", stdlib, err)
				return err
			}

			results = append(results, Result{stdlib, importedBy})
			return nil
		})
	}

	if err := sg.Wait(); err != nil {
		fmt.Println(err)
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Num > results[j].Num })

	j, err := json.Marshal(results)
	if err != nil {
		fmt.Printf("failed to marshal: %q\n", err)
		return
	}
	err = os.WriteFile("result.json", j, 0644)
	if err != nil {
		fmt.Printf("failed to write file: %q\n", err)
		return
	}

	fmt.Println("DONE!")
}

func removeInternalPkg(libs []string) []string {
	var ret []string
	for _, lib := range libs {
		var isInternal bool
		for _, d := range strings.Split(lib, "/") {
			if d == "internal" {
				isInternal = true
			}
		}
		if !isInternal {
			ret = append(ret, lib)
		}
	}

	return ret
}

// extractImportedBy returns `imported-by` value.
// For example, return 19,638 if given below
//
//   <span class="go-Main-headerDetailItem" data-test-id="UnitHeader-importedby">
//     <a href="/runtime/debug?tab=importedby" aria-label="Go to Imported By"
//         data-gtmc="header link">
//        <span class="go-textSubtle">Imported by: </span>19,638
//     </a>
//   </span>
//
//   SPAN
//     TEXT
//     A
//       TEXT
//       SPAN
//       TEXT
func extractImportedBy(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "span" {
		for _, attr := range n.Attr {
			if attr.Key == "data-test-id" && attr.Val == "UnitHeader-importedby" {
				return n.FirstChild.NextSibling.FirstChild.NextSibling.NextSibling.Data
			}
		}
	}

	var ret string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if ret2 := extractImportedBy(c); ret2 != "" {
			ret = ret2
			break
		}
	}

	return ret
}
