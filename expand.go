package layercfg

import (
	"fmt"
	"sort"
	"strings"
)

// Override records that env replaced a value from the common file.
type Override struct {
	Key        string
	CommonFile string
	CommonLine int
	EnvFile    string
	EnvLine    int
}

// Merge combines common and env entries; env wins on conflicts.
// Returns merged entries keyed by name plus override records.
func Merge(common, env []Entry) (map[string]Entry, []Override) {
	merged := map[string]Entry{}
	var overrides []Override
	for _, e := range common {
		merged[e.Key] = e
	}
	for _, e := range env {
		if prev, ok := merged[e.Key]; ok {
			overrides = append(overrides, Override{
				Key:        e.Key,
				CommonFile: prev.File,
				CommonLine: prev.Line,
				EnvFile:    e.File,
				EnvLine:    e.Line,
			})
		}
		merged[e.Key] = e
	}
	return merged, overrides
}

// Refs returns the ${NAME} references in a value that participates in
// expansion (bare or double-quoted). Single-quoted values have none.
func Refs(e Entry) []string {
	if e.Kind == KindSingle {
		return nil
	}
	var refs []string
	s := e.Value
	for i := 0; i < len(s); i++ {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				continue
			}
			name := s[i+2 : i+2+end]
			if name != "" {
				refs = append(refs, name)
			}
			i += 2 + end
		}
	}
	return refs
}

// ExpandError reports a reference resolution failure.
type ExpandError struct {
	Key string
	Msg string
}

func (e *ExpandError) Error() string {
	return fmt.Sprintf("key %s: %s", e.Key, e.Msg)
}

// Expand resolves ${NAME} references across the merged entries.
// Returns expanded values and per-key dependency lists (sorted keys).
func Expand(merged map[string]Entry) (map[string]string, map[string][]string, error) {
	values := map[string]string{}
	deps := map[string][]string{}
	state := map[string]int{} // 0=unvisited 1=in progress 2=done

	var resolve func(key string) error
	resolve = func(key string) error {
		switch state[key] {
		case 2:
			return nil
		case 1:
			return &ExpandError{key, "circular reference detected"}
		}
		e, ok := merged[key]
		if !ok {
			return &ExpandError{key, "reference to undefined key"}
		}
		state[key] = 1
		refs := Refs(e)
		seen := map[string]bool{}
		var depList []string
		for _, r := range refs {
			if _, ok := merged[r]; !ok {
				return &ExpandError{key, fmt.Sprintf("reference ${%s} is not defined", r)}
			}
			if err := resolve(r); err != nil {
				return err
			}
			if !seen[r] {
				seen[r] = true
				depList = append(depList, r)
			}
		}
		if e.Kind == KindSingle {
			values[key] = e.Value
		} else {
			values[key] = substitute(e.Value, values)
		}
		sort.Strings(depList)
		deps[key] = depList
		state[key] = 2
		return nil
	}

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := resolve(k); err != nil {
			return nil, nil, err
		}
	}
	return values, deps, nil
}

func substitute(s string, values map[string]string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			end := strings.IndexByte(s[i+2:], '}')
			if end >= 0 {
				name := s[i+2 : i+2+end]
				if v, ok := values[name]; ok {
					b.WriteString(v)
					i += 2 + end
					continue
				}
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
