package layercfg

import (
	"fmt"
	"strings"
)

// ValueKind describes how a value was written in the source file.
type ValueKind int

const (
	KindBare ValueKind = iota
	KindSingle
	KindDouble
)

// Entry is a single parsed key/value definition.
type Entry struct {
	Key   string
	Value string
	Kind  ValueKind
	File  string
	Line  int
}

// ParseError locates a problem in a source file.
type ParseError struct {
	File string
	Line int
	Msg  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg)
}

func isKeyStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isKeyChar(c byte) bool {
	return isKeyStart(c) || (c >= '0' && c <= '9')
}

// ParseFile parses one parameters file into entries in file order.
// Duplicate keys within the same file are an error.
func ParseFile(name string, content []byte) ([]Entry, error) {
	var entries []Entry
	seen := map[string]int{}
	text := strings.TrimPrefix(string(content), "\ufeff")
	lines := strings.Split(text, "\n")
	for i, raw := range lines {
		lineNo := i + 1
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !isKeyStart(trimmed[0]) {
			return nil, &ParseError{name, lineNo, "invalid key: must start with a letter or underscore"}
		}
		j := 0
		for j < len(trimmed) && isKeyChar(trimmed[j]) {
			j++
		}
		key := trimmed[:j]
		rest := strings.TrimSpace(trimmed[j:])
		if !strings.HasPrefix(rest, "=") {
			return nil, &ParseError{name, lineNo, fmt.Sprintf("key %q: expected '=' after key", key)}
		}
		if prev, ok := seen[key]; ok {
			return nil, &ParseError{name, lineNo, fmt.Sprintf("duplicate key %q (first defined on line %d)", key, prev)}
		}
		seen[key] = lineNo
		value, kind, err := parseValue(name, lineNo, strings.TrimSpace(rest[1:]))
		if err != nil {
			return nil, err
		}
		entries = append(entries, Entry{Key: key, Value: value, Kind: kind, File: name, Line: lineNo})
	}
	return entries, nil
}

func parseValue(file string, line int, s string) (string, ValueKind, error) {
	if s == "" {
		return "", KindBare, nil
	}
	switch s[0] {
	case '\'':
		end := strings.IndexByte(s[1:], '\'')
		if end < 0 {
			return "", KindBare, &ParseError{file, line, "unterminated single-quoted value"}
		}
		if tail := strings.TrimSpace(s[1+end+1:]); tail != "" {
			return "", KindBare, &ParseError{file, line, "unexpected characters after closing quote"}
		}
		return s[1 : 1+end], KindSingle, nil
	case '"':
		var b strings.Builder
		i := 1
		for i < len(s) {
			c := s[i]
			switch c {
			case '"':
				if tail := strings.TrimSpace(s[i+1:]); tail != "" {
					return "", KindBare, &ParseError{file, line, "unexpected characters after closing quote"}
				}
				return b.String(), KindDouble, nil
			case '\\':
				if i+1 >= len(s) {
					return "", KindBare, &ParseError{file, line, "unterminated escape at end of double-quoted value"}
				}
				switch s[i+1] {
				case '\\':
					b.WriteByte('\\')
				case '"':
					b.WriteByte('"')
				case 'n':
					b.WriteByte('\n')
				default:
					return "", KindBare, &ParseError{file, line, fmt.Sprintf("unsupported escape \\%c in double-quoted value", s[i+1])}
				}
				i += 2
			default:
				b.WriteByte(c)
				i++
			}
		}
		return "", KindBare, &ParseError{file, line, "unterminated double-quoted value"}
	default:
		return strings.TrimSpace(s), KindBare, nil
	}
}
