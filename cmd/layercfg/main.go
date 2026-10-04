package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	layercfg "example.com/layered-deployment-config"
)

const usageText = `usage:
  layercfg compile <common.params> <env.params> <sensitive.json>
  layercfg explain <common.params> <env.params> <sensitive.json>

compile prints the merged JSON config with real values.
explain prints a review report with sensitive values masked.`

func usage() {
	fmt.Fprintln(os.Stderr, usageText)
	os.Exit(2)
}

func load(path string) ([]layercfg.Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return layercfg.ParseFile(path, data)
}

func loadSensitive(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []string
	if err := json.Unmarshal([]byte(strings.TrimPrefix(string(data), "\ufeff")), &list); err != nil {
		return nil, fmt.Errorf("%s: sensitive list must be a JSON array of key names: %w", path, err)
	}
	return list, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func main() {
	if len(os.Args) != 5 || (os.Args[1] != "compile" && os.Args[1] != "explain") {
		usage()
	}
	mode := os.Args[1]

	common, err := load(os.Args[2])
	if err != nil {
		fail(err)
	}
	env, err := load(os.Args[3])
	if err != nil {
		fail(err)
	}
	sensitiveList, err := loadSensitive(os.Args[4])
	if err != nil {
		fail(err)
	}

	merged, overrides := layercfg.Merge(common, env)
	values, deps, err := layercfg.Expand(merged)
	if err != nil {
		fail(err)
	}
	sensitive := layercfg.SensitiveKeys(sensitiveList, deps)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if mode == "compile" {
		err = enc.Encode(layercfg.Config(values))
	} else {
		err = enc.Encode(layercfg.BuildReport(merged, values, deps, overrides, sensitive))
	}
	if err != nil {
		fail(err)
	}
}
