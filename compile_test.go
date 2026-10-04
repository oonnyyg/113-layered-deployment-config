package main

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, name, content string) []Entry {
	t.Helper()
	entries, err := parseFile(name, content)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return entries
}

func TestCrossFileOverride(t *testing.T) {
	common := mustParse(t, "common.conf", `
# 公共参数
HOST = example.com
PORT = 8080
URL = http://${HOST}:${PORT}
`)
	env := mustParse(t, "prod.conf", `
HOST = prod.internal
DEBUG = false
`)
	res, err := Compile(common, env, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Values["URL"]; got != "http://prod.internal:8080" {
		t.Errorf("URL = %q, want override applied", got)
	}
	r := res.Report["HOST"]
	if r.Source != "prod.conf" || len(r.Overrides) != 1 || r.Overrides[0].From != "common.conf" {
		t.Errorf("unexpected HOST report: %+v", r)
	}
	if deps := res.Report["URL"].DependsOn; len(deps) != 2 {
		t.Errorf("URL deps = %v", deps)
	}
}

func TestForwardAndIndirectReference(t *testing.T) {
	common := mustParse(t, "c.conf", `
A = ${B}-x
B = ${C}-y
C = base
`)
	res, err := Compile(common, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Values["A"] != "base-y-x" {
		t.Errorf("A = %q", res.Values["A"])
	}
}

func TestCircularReference(t *testing.T) {
	common := mustParse(t, "c.conf", "A = ${B}\nB = ${A}\n")
	if _, err := Compile(common, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "circular") {
		t.Fatalf("want circular error, got %v", err)
	}
}

func TestUndefinedReference(t *testing.T) {
	common := mustParse(t, "c.conf", "A = ${MISSING}\n")
	if _, err := Compile(common, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "undefined") {
		t.Fatalf("want undefined error, got %v", err)
	}
}

func TestIndirectSensitivity(t *testing.T) {
	common := mustParse(t, "c.conf", `
PASSWORD = s3cret
DSN = "postgres://u:${PASSWORD}@db/app"
LITERAL = 'raw ${PASSWORD} text'
PLAIN = hello
`)
	res, err := Compile(common, nil, map[string]bool{"PASSWORD": true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Values["DSN"] != "postgres://u:s3cret@db/app" {
		t.Errorf("DSN = %q", res.Values["DSN"])
	}
	if res.Values["LITERAL"] != "raw ${PASSWORD} text" {
		t.Errorf("single-quoted value must stay literal, got %q", res.Values["LITERAL"])
	}
	if !res.Report["DSN"].Sensitive {
		t.Error("DSN must inherit sensitivity through reference")
	}
	if res.Report["LITERAL"].Sensitive {
		t.Error("single-quoted reference text must not create dependency")
	}
	if res.Report["PLAIN"].Sensitive {
		t.Error("unrelated key must not be sensitive")
	}
}

func TestDuplicateKeySameFile(t *testing.T) {
	_, err := parseFile("c.conf", "A = 1\nA = 2\n")
	if err == nil || !strings.Contains(err.Error(), "c.conf:2") {
		t.Fatalf("want located duplicate error, got %v", err)
	}
}

func TestQuotingAndEscapes(t *testing.T) {
	entries := mustParse(t, "c.conf", `
A =   spaced out  
B = '  keep  '
C = "a\nb \"q\" \\"
`)
	if entries[0].Value != "spaced out" {
		t.Errorf("A = %q", entries[0].Value)
	}
	if entries[1].Value != "  keep  " {
		t.Errorf("B = %q", entries[1].Value)
	}
	if entries[2].Value != "a\nb \"q\" \\" {
		t.Errorf("C = %q", entries[2].Value)
	}
}

func TestParseErrorLocations(t *testing.T) {
	cases := []struct {
		content string
		want    string
	}{
		{"A = \"unterminated\n", "c.conf:1"},
		{"A = \"bad \\t escape\"\n", "c.conf:1"},
		{"A = 'unterminated\n", "c.conf:1"},
		{"1BAD = x\n", "c.conf:1"},
		{"A = 'x' trailing\n", "c.conf:1"},
	}
	for _, tc := range cases {
		if _, err := parseFile("c.conf", tc.content); err == nil ||
			!strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: want error containing %q, got %v", tc.content, tc.want, err)
		}
	}
}
