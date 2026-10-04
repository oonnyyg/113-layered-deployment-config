package main

import (
	"fmt"
	"strings"
)

// Entry 是解析后的一条键值定义。
type Entry struct {
	Key      string
	Value    string
	Quoted   byte // 0 表示未加引号，' 或 " 表示引号风格
	File     string
	Line     int
	Refs     []string // 解析期即可确定的 ${NAME} 引用（单引号值不产生引用）
}

// ParseError 携带文件与行号的解析错误。
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

// parseFile 按行解析参数文件内容（UTF-8）。
func parseFile(name string, content string) ([]Entry, error) {
	var entries []Entry
	seen := map[string]int{}
	lines := strings.Split(content, "\n")
	for i, raw := range lines {
		lineNo := i + 1
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		entry, err := parseLine(name, lineNo, trimmed)
		if err != nil {
			return nil, err
		}
		if prev, dup := seen[entry.Key]; dup {
			return nil, &ParseError{File: name, Line: lineNo,
				Msg: fmt.Sprintf("duplicate key %q (first defined at line %d)", entry.Key, prev)}
		}
		seen[entry.Key] = lineNo
		entries = append(entries, entry)
	}
	return entries, nil
}

func parseLine(file string, lineNo int, line string) (Entry, error) {
	entry := Entry{File: file, Line: lineNo}
	i := 0
	if !isKeyStart(line[0]) {
		return entry, &ParseError{File: file, Line: lineNo, Msg: "key must start with a letter or underscore"}
	}
	for i < len(line) && isKeyChar(line[i]) {
		i++
	}
	entry.Key = line[:i]
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	if i >= len(line) || line[i] != '=' {
		return entry, &ParseError{File: file, Line: lineNo, Msg: fmt.Sprintf("expected '=' after key %q", entry.Key)}
	}
	i++
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	rest := line[i:]
	switch {
	case strings.HasPrefix(rest, "'"):
		body := rest[1:]
		end := strings.IndexByte(body, '\'')
		if end < 0 {
			return entry, &ParseError{File: file, Line: lineNo, Msg: "unterminated single-quoted value"}
		}
		if tail := strings.TrimSpace(body[end+1:]); tail != "" {
			return entry, &ParseError{File: file, Line: lineNo, Msg: "unexpected characters after closing quote"}
		}
		entry.Quoted = '\''
		entry.Value = body[:end]
	case strings.HasPrefix(rest, "\""):
		body := rest[1:]
		var sb strings.Builder
		closed := false
		for j := 0; j < len(body); j++ {
			c := body[j]
			if c == '"' {
				if tail := strings.TrimSpace(body[j+1:]); tail != "" {
					return entry, &ParseError{File: file, Line: lineNo, Msg: "unexpected characters after closing quote"}
				}
				closed = true
				break
			}
			if c == '\\' {
				if j+1 >= len(body) {
					return entry, &ParseError{File: file, Line: lineNo, Msg: "unterminated double-quoted value"}
				}
				j++
				switch body[j] {
				case '\\':
					sb.WriteByte('\\')
				case '"':
					sb.WriteByte('"')
				case 'n':
					sb.WriteByte('\n')
				default:
					return entry, &ParseError{File: file, Line: lineNo,
						Msg: fmt.Sprintf("unsupported escape sequence \\%c", body[j])}
				}
				continue
			}
			sb.WriteByte(c)
		}
		if !closed {
			return entry, &ParseError{File: file, Line: lineNo, Msg: "unterminated double-quoted value"}
		}
		entry.Quoted = '"'
		entry.Value = sb.String()
		entry.Refs = findRefs(entry.Value)
	default:
		value := strings.TrimSpace(rest)
		if strings.ContainsAny(value, "'\"") {
			return entry, &ParseError{File: file, Line: lineNo, Msg: "quote characters must wrap the whole value"}
		}
		entry.Value = value
		entry.Refs = findRefs(value)
	}
	return entry, nil
}

// findRefs 提取值中的 ${NAME} 引用名。
func findRefs(value string) []string {
	var refs []string
	for i := 0; i < len(value); i++ {
		if value[i] == '$' && i+1 < len(value) && value[i+1] == '{' {
			end := strings.IndexByte(value[i+2:], '}')
			if end >= 0 {
				refs = append(refs, value[i+2:i+2+end])
				i += 2 + end
			}
		}
	}
	return refs
}
