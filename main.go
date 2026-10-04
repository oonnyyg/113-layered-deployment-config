package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "compile":
		err = run(true, os.Args[2:])
	case "explain":
		err = run(false, os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  cfgc compile -common FILE -env FILE -sensitive FILE   输出合并后的 JSON 配置
  cfgc explain -common FILE -env FILE -sensitive FILE   输出每个键的来源/覆盖/依赖审查报告`)
}

func run(configOnly bool, args []string) error {
	fs := flag.NewFlagSet("cfgc", flag.ContinueOnError)
	commonPath := fs.String("common", "", "公共参数文件 (UTF-8)")
	envPath := fs.String("env", "", "环境覆盖参数文件 (UTF-8)")
	sensitivePath := fs.String("sensitive", "", "敏感键名单 JSON 文件（字符串数组）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *commonPath == "" || *envPath == "" || *sensitivePath == "" {
		return fmt.Errorf("-common, -env and -sensitive are all required")
	}

	common, err := loadEntries(*commonPath)
	if err != nil {
		return err
	}
	env, err := loadEntries(*envPath)
	if err != nil {
		return err
	}
	sensitive, err := loadSensitive(*sensitivePath)
	if err != nil {
		return err
	}

	res, err := Compile(common, env, sensitive)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if configOnly {
		return enc.Encode(res.Values)
	}
	return enc.Encode(res.Report)
}

func loadEntries(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseFile(path, string(data))
}

func loadSensitive(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	return set, nil
}
