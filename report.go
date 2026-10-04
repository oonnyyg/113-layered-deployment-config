package layercfg

import "sort"

// Masked is the placeholder used for sensitive values in reports.
const Masked = "REDACTED"

// KeyReport describes one key in the explain output.
type KeyReport struct {
	Key          string   `json:"key"`
	Value        string   `json:"value"`
	Sensitive    bool     `json:"sensitive"`
	Kind         string   `json:"kind"`
	File         string   `json:"file"`
	Line         int      `json:"line"`
	Dependencies []string `json:"dependencies"`
	Overridden   *Source  `json:"overridden,omitempty"`
}

// Source points at a definition that was replaced by an override.
type Source struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

// Report is the full explain output.
type Report struct {
	Keys []KeyReport `json:"keys"`
}

func kindName(k ValueKind) string {
	switch k {
	case KindSingle:
		return "single-quoted"
	case KindDouble:
		return "double-quoted"
	default:
		return "bare"
	}
}

// SensitiveKeys computes the transitive sensitive set: keys listed in the
// sensitive list plus every key that (transitively) depends on them.
func SensitiveKeys(sensitiveList []string, deps map[string][]string) map[string]bool {
	sensitive := map[string]bool{}
	for _, k := range sensitiveList {
		sensitive[k] = true
	}
	dependents := map[string][]string{}
	for k, ds := range deps {
		for _, d := range ds {
			dependents[d] = append(dependents[d], k)
		}
	}
	queue := append([]string(nil), sensitiveList...)
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		for _, dep := range dependents[k] {
			if !sensitive[dep] {
				sensitive[dep] = true
				queue = append(queue, dep)
			}
		}
	}
	return sensitive
}

// BuildReport assembles the explain output with sensitive values masked.
func BuildReport(merged map[string]Entry, values map[string]string, deps map[string][]string, overrides []Override, sensitive map[string]bool) Report {
	overriddenBy := map[string]Override{}
	for _, o := range overrides {
		overriddenBy[o.Key] = o
	}
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	report := Report{Keys: make([]KeyReport, 0, len(keys))}
	for _, k := range keys {
		e := merged[k]
		v := values[k]
		if sensitive[k] {
			v = Masked
		}
		kr := KeyReport{
			Key:          k,
			Value:        v,
			Sensitive:    sensitive[k],
			Kind:         kindName(e.Kind),
			File:         e.File,
			Line:         e.Line,
			Dependencies: deps[k],
		}
		if kr.Dependencies == nil {
			kr.Dependencies = []string{}
		}
		if o, ok := overriddenBy[k]; ok {
			kr.Overridden = &Source{File: o.CommonFile, Line: o.CommonLine}
		}
		report.Keys = append(report.Keys, kr)
	}
	return report
}

// Config returns the final key/value map for compile output (real values).
func Config(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
