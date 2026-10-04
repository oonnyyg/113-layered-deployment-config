package layercfg

import (
	"strings"
	"testing"
)

func parse(t *testing.T, name, content string) []Entry {
	t.Helper()
	entries, err := ParseFile(name, []byte(content))
	if err != nil {
		t.Fatalf("ParseFile(%s) failed: %v", name, err)
	}
	return entries
}

func build(t *testing.T, common, env string, sensitive []string) (map[string]string, Report) {
	t.Helper()
	merged, overrides := Merge(parse(t, "common.params", common), parse(t, "env.params", env))
	values, deps, err := Expand(merged)
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}
	sens := SensitiveKeys(sensitive, deps)
	return values, BuildReport(merged, values, deps, overrides, sens)
}

func reportValue(t *testing.T, r Report, key string) KeyReport {
	t.Helper()
	for _, k := range r.Keys {
		if k.Key == key {
			return k
		}
	}
	t.Fatalf("key %s not in report", key)
	return KeyReport{}
}

func TestCrossFileOverride(t *testing.T) {
	values, report := build(t,
		"HOST = internal.example.com\nPORT = 8080\n",
		"PORT = 9090\n",
		nil)
	if values["PORT"] != "9090" {
		t.Fatalf("PORT = %q, want 9090", values["PORT"])
	}
	kr := reportValue(t, report, "PORT")
	if kr.Overridden == nil || kr.Overridden.File != "common.params" || kr.Overridden.Line != 2 {
		t.Fatalf("override record wrong: %+v", kr.Overridden)
	}
	if kr.File != "env.params" || kr.Line != 1 {
		t.Fatalf("source wrong: %+v", kr)
	}
}

func TestForwardAndIndirectRefs(t *testing.T) {
	values, report := build(t,
		"URL = https://${HOST}:${PORT}/api\nHOST = ${BASE}.example.com\nBASE = 'app'\nPORT = 443\n",
		"",
		nil)
	if values["URL"] != "https://app.example.com:443/api" {
		t.Fatalf("URL = %q", values["URL"])
	}
	kr := reportValue(t, report, "URL")
	if strings.Join(kr.Dependencies, ",") != "HOST,PORT" {
		t.Fatalf("deps = %v", kr.Dependencies)
	}
	// single-quoted value keeps literal text and has no deps
	if kr2 := reportValue(t, report, "BASE"); len(kr2.Dependencies) != 0 {
		t.Fatalf("BASE deps = %v", kr2.Dependencies)
	}
}

func TestSingleQuotedRefNotExpanded(t *testing.T) {
	values, _ := build(t, "A = 1\nB = '${A}'\n", "", nil)
	if values["B"] != "${A}" {
		t.Fatalf("B = %q, want literal ${A}", values["B"])
	}
}

func TestDoubleQuotedEscapes(t *testing.T) {
	values, _ := build(t, `MSG = "line\nwith \"quote\" and \\ slash"`, "", nil)
	if values["MSG"] != "line\nwith \"quote\" and \\ slash" {
		t.Fatalf("MSG = %q", values["MSG"])
	}
}

func TestCycleDetected(t *testing.T) {
	merged, _ := Merge(parse(t, "c", "A = ${B}\nB = ${A}\n"), nil)
	_, _, err := Expand(merged)
	if err == nil || !strings.Contains(err.Error(), "circular") {
		t.Fatalf("want circular error, got %v", err)
	}
}

func TestMissingRef(t *testing.T) {
	merged, _ := Merge(parse(t, "c", "A = ${NOPE}\n"), nil)
	_, _, err := Expand(merged)
	if err == nil || !strings.Contains(err.Error(), "not defined") {
		t.Fatalf("want undefined-ref error, got %v", err)
	}
}

func TestIndirectSensitiveMasking(t *testing.T) {
	values, report := build(t,
		"PASSWORD = s3cret\nDSN = postgres://u:${PASSWORD}@db/app\nDESC = \"uses ${DSN}\"\n",
		"",
		[]string{"PASSWORD"})
	if values["DSN"] != "postgres://u:s3cret@db/app" {
		t.Fatalf("compile output must keep real value, got %q", values["DSN"])
	}
	for _, key := range []string{"PASSWORD", "DSN", "DESC"} {
		kr := reportValue(t, report, key)
		if !kr.Sensitive || kr.Value != Masked {
			t.Fatalf("%s should be masked, got %+v", key, kr)
		}
	}
	if strings.Contains(reportJSON(t, report), "s3cret") {
		t.Fatal("report leaks sensitive value")
	}
}

func reportJSON(t *testing.T, r Report) string {
	t.Helper()
	var b strings.Builder
	for _, k := range r.Keys {
		b.WriteString(k.Value)
	}
	return b.String()
}

func TestDuplicateKeySameFile(t *testing.T) {
	_, err := ParseFile("f", []byte("A = 1\nA = 2\n"))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("want duplicate error, got %v", err)
	}
}

func TestParseErrorsLocated(t *testing.T) {
	cases := []string{
		"A = 'unterminated\n",
		"A = \"unterminated\n",
		"A = \"bad \\t escape\"\n",
		"1BAD = 1\n",
		"A 1\n",
	}
	for _, c := range cases {
		_, err := ParseFile("cfg.params", []byte(c))
		if err == nil {
			t.Fatalf("want error for %q", c)
		}
		if !strings.Contains(err.Error(), "cfg.params:1:") {
			t.Fatalf("error lacks file:line: %v", err)
		}
	}
}

func TestCommentsAndBlankLines(t *testing.T) {
	entries := parse(t, "f", "# comment\n\n   # indented\nKEY = v # not a comment\n")
	if len(entries) != 1 || entries[0].Value != "v # not a comment" {
		t.Fatalf("entries = %+v", entries)
	}
}
