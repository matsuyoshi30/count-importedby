package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/fatih/semgroup"
	"golang.org/x/tools/go/packages"
)

var stdlibs []string

const apiURL = "https://api.godoc.org/importers/"

type APIResp struct {
	Results []Result `json:"results"`
}

type Result struct {
	Path     string `json:"path"`
	Synopsis string `json:"synopsis"`
}

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

func main() {
	resp, err := http.Get(apiURL)
	if err != nil {
		fmt.Println(err)
		return
	} else if resp.StatusCode != http.StatusOK {
		fmt.Println("api.godoc.org returns not 200")
		return
	}

	results := make(map[string]int)

	sg := semgroup.NewGroup(context.Background(), 20)
	for _, stdlib := range removeInternalPkg(stdlibs) {
		stdlib := stdlib
		sg.Go(func() error {
			log.Printf("[INFO] start request about package %s\n", stdlib)
			resp, err := http.Get(apiURL + stdlib)
			log.Printf("[INFO] finish request about package %s\n", stdlib)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("[ERR ] package %s: %q\n", stdlib, err)
				return err
			}

			var apiResp APIResp
			err = json.Unmarshal(body, &apiResp)
			if err != nil {
				log.Printf("[ERR ] package %s: %q\n", stdlib, err)
				return err
			}

			results[stdlib] = len(apiResp.Results)
			return nil
		})
	}

	if err := sg.Wait(); err != nil {
		log.Println("something happened")
	}

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
