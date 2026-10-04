package main

import (
	"fmt"
	"sort"
	"strings"
)

// Override 记录一次覆盖：键先出现在 From，后被 To 中的定义取代。
type Override struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// KeyReport 是解释输出中单个键的审查信息，不包含敏感值。
type KeyReport struct {
	Source    string     `json:"source"`
	Line      int        `json:"line"`
	Overrides []Override `json:"overrides,omitempty"`
	DependsOn []string   `json:"depends_on,omitempty"`
	Sensitive bool       `json:"sensitive"`
}

// Result 是编译结果：最终配置与审查报告。
type Result struct {
	Values map[string]string
	Report map[string]*KeyReport
}

// Compile 合并公共与环境定义，展开引用并传播敏感标记。
func Compile(common, env []Entry, sensitive map[string]bool) (*Result, error) {
	final := map[string]Entry{}
	report := map[string]*KeyReport{}
	order := []string{}

	apply := func(entries []Entry) {
		for _, e := range entries {
			if _, exists := final[e.Key]; !exists {
				order = append(order, e.Key)
				report[e.Key] = &KeyReport{Source: e.File, Line: e.Line}
			} else {
				r := report[e.Key]
				r.Overrides = append(r.Overrides, Override{From: r.Source, To: e.File})
				r.Source = e.File
				r.Line = e.Line
			}
			final[e.Key] = e
		}
	}
	apply(common)
	apply(env)

	deps := map[string][]string{}
	for key, e := range final {
		seen := map[string]bool{}
		for _, ref := range e.Refs {
			if _, ok := final[ref]; !ok {
				return nil, fmt.Errorf("%s:%d: key %q references undefined ${%s}",
					e.File, e.Line, key, ref)
			}
			if !seen[ref] {
				seen[ref] = true
				deps[key] = append(deps[key], ref)
			}
		}
	}

	values := map[string]string{}
	state := map[string]int{} // 0 未访问 1 进行中 2 完成
	var expand func(key string, stack []string) error
	expand = func(key string, stack []string) error {
		switch state[key] {
		case 2:
			return nil
		case 1:
			cycle := append(stack, key)
			return fmt.Errorf("circular reference detected: %s", strings.Join(cycle, " -> "))
		}
		state[key] = 1
		stack = append(stack, key)
		for _, dep := range deps[key] {
			if err := expand(dep, stack); err != nil {
				return err
			}
		}
		e := final[key]
		if e.Quoted == '\'' {
			values[key] = e.Value
		} else {
			values[key] = substitute(e.Value, values)
		}
		state[key] = 2
		return nil
	}
	for _, key := range order {
		if err := expand(key, nil); err != nil {
			return nil, err
		}
	}

	memo := map[string]bool{}
	var mark func(key string) bool
	mark = func(key string) bool {
		if v, ok := memo[key]; ok {
			return v
		}
		memo[key] = false
		s := sensitive[key]
		for _, dep := range deps[key] {
			if mark(dep) {
				s = true
			}
		}
		memo[key] = s
		return s
	}
	sensitiveKeys := map[string]bool{}
	for key := range final {
		if mark(key) {
			sensitiveKeys[key] = true
		}
	}

	res := &Result{Values: values, Report: map[string]*KeyReport{}}
	for _, key := range order {
		r := report[key]
		r.DependsOn = deps[key]
		r.Sensitive = sensitiveKeys[key]
		res.Report[key] = r
	}
	return res, nil
}

// substitute 用已解析的值替换 ${NAME}。
func substitute(value string, values map[string]string) string {
	var sb strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '$' && i+1 < len(value) && value[i+1] == '{' {
			if end := strings.IndexByte(value[i+2:], '}'); end >= 0 {
				sb.WriteString(values[value[i+2:i+2+end]])
				i += 2 + end
				continue
			}
		}
		sb.WriteByte(value[i])
	}
	return sb.String()
}

// SortedKeys 返回稳定排序的键名。
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
