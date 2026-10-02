package config

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetect(t *testing.T) {
	cases := map[string]Format{
		".env":             Env,
		".env.production":  Env,
		"config/prod.env":  Env,
		"values.yaml":      YAML,
		"overlays/app.YML": YAML,
		"settings.json":    JSON,
	}
	for path, want := range cases {
		got, err := Detect(path)
		if err != nil || got != want {
			t.Errorf("Detect(%q) = %q, %v; want %q", path, got, err, want)
		}
	}
	if _, err := Detect("config.toml"); err == nil {
		t.Error("Detect(config.toml) should fail")
	}
}

func TestParseEnv(t *testing.T) {
	src := strings.Join([]string{
		"# comment",
		"",
		"PLAIN=value",
		"export EXPORTED=yes",
		"SPACED = padded  ",
		"EMPTY=",
		"INLINE=keep # dropped",
		"URL=https://example.com/#anchor",
		`DOUBLE="a \"quoted\" line\nnext"`,
		"SINGLE='$NOT_EXPANDED \\n'",
		`MULTI="first`,
		`second" # trailing comment`,
		"REPEATED=1",
		"REPEATED=2",
		"CRLF=x\r",
		"HTML=<a&b>",
	}, "\n")
	got, err := parseEnv([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := Values{
		"PLAIN":    `"value"`,
		"EXPORTED": `"yes"`,
		"SPACED":   `"padded"`,
		"EMPTY":    `""`,
		"INLINE":   `"keep"`,
		"URL":      `"https://example.com/#anchor"`,
		"DOUBLE":   `"a \"quoted\" line\nnext"`,
		"SINGLE":   `"$NOT_EXPANDED \\n"`,
		"MULTI":    `"first\nsecond"`,
		"REPEATED": `"2"`,
		"CRLF":     `"x"`,
		"HTML":     `"<a&b>"`,
	}
	if !maps.Equal(got, want) {
		t.Errorf("parseEnv:\n got %v\nwant %v", got, want)
	}
}

func TestParseEnvErrors(t *testing.T) {
	for _, src := range []string{
		"NO_EQUALS",
		"=value",
		"TWO WORDS=x",
		`OPEN="never closed`,
		"OPEN='never closed",
		`AFTER="x" junk`,
	} {
		if _, err := parseEnv([]byte(src)); err == nil {
			t.Errorf("parseEnv(%q) should fail", src)
		}
	}
}

func TestParseYAML(t *testing.T) {
	src := `
db:
  host: localhost
  port: 5432
  port_str: "5432"
  replicas: [a, b]
features: {}
tags: []
labels:
  app.kubernetes.io/name: api
nothing: null
`
	got, err := parseYAML([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := Values{
		"db.host":                          `"localhost"`,
		"db.port":                          `5432`,
		"db.port_str":                      `"5432"`,
		"db.replicas[0]":                   `"a"`,
		"db.replicas[1]":                   `"b"`,
		"features":                         `{}`,
		"tags":                             `[]`,
		`labels["app.kubernetes.io/name"]`: `"api"`,
		"nothing":                          `null`,
	}
	if !maps.Equal(got, want) {
		t.Errorf("parseYAML:\n got %v\nwant %v", got, want)
	}
}

func TestParseYAMLConfigMaps(t *testing.T) {
	src := `
apiVersion: apps/v1
kind: Deployment
metadata: {name: api}
---
apiVersion: v1
kind: ConfigMap
metadata: {name: api-env}
data:
  LOG_LEVEL: info
  PORT: "8080"
---
apiVersion: v1
kind: ConfigMap
metadata: {name: api-files}
binaryData:
  cert.der: AAEC
`
	got, err := parseYAML([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := Values{"LOG_LEVEL": `"info"`, "PORT": `"8080"`, "cert.der": `"AAEC"`}
	if !maps.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseYAMLErrors(t *testing.T) {
	for name, src := range map[string]string{
		"duplicate ConfigMap key": "kind: ConfigMap\ndata: {A: x}\n---\nkind: ConfigMap\ndata: {A: y}\n",
		"several plain documents": "a: 1\n---\nb: 2\n",
		"invalid syntax":          "a: [unclosed\n",
	} {
		if _, err := parseYAML([]byte(src)); err == nil {
			t.Errorf("%s: should fail", name)
		}
	}
}

func TestParseJSON(t *testing.T) {
	got, err := parseJSON([]byte(`{"id": 9007199254740993, "ratio": 0.5, "on": true, "list": [{"k": "v"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := Values{
		"id":        `9007199254740993`, // exact, not rounded through float64
		"ratio":     `0.5`,
		"on":        `true`,
		"list[0].k": `"v"`,
	}
	if !maps.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if _, err := parseJSON([]byte(`{} {}`)); err == nil {
		t.Error("trailing data should fail")
	}
}

// The same settings written in YAML and JSON must flatten identically, or
// comparing a JSON environment to a YAML one would report false drift.
func TestYAMLAndJSONAgree(t *testing.T) {
	y, err := parseYAML([]byte("a:\n  b: 1\n  c: [true, x]\n  d: 2024-01-01\n"))
	if err != nil {
		t.Fatal(err)
	}
	j, err := parseJSON([]byte(`{"a": {"b": 1, "c": [true, "x"], "d": "2024-01-01"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(y, j) {
		t.Errorf("yaml %v != json %v", y, j)
	}
}

func TestLoadNamesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.env")
	if err := os.WriteFile(path, []byte("oops"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "line 1") {
		t.Errorf("error should name file and line, got %v", err)
	}
}

func TestParseYAMLAnchorsAndMerge(t *testing.T) {
	got, err := parseYAML([]byte("base: &b {pool: 5, at: 2024-01-01}\nprod:\n  <<: *b\n  pool: 20\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := Values{
		"base.pool": `5`, "base.at": `"2024-01-01"`,
		"prod.pool": `20`, "prod.at": `"2024-01-01"`,
	}
	if !maps.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The fast paths in canonical must match the JSON encoder byte for byte, or
// the same value read two ways would report drift.
func TestCanonicalFastPathsMatchEncoder(t *testing.T) {
	for _, v := range []any{
		nil, true, false, 0, -42, 1 << 40, json.Number("1.50"),
		"", "plain", "with space ~!@#$%^*()", "<a&b>", `q"uote`, `back\slash`,
		"tab\t", "nl\n", "\x7f", "ünïcode", " ", "\xff",
	} {
		var b strings.Builder
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			t.Fatal(err)
		}
		if want := strings.TrimSuffix(b.String(), "\n"); canonical(v) != want {
			t.Errorf("canonical(%#v) = %s, encoder gives %s", v, canonical(v), want)
		}
	}
}

func TestParseMatchesLoad(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "k8s", "staging", "configmap.yaml")
	want, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse("configmap.yaml", data)
	if err != nil || !maps.Equal(got, want) {
		t.Errorf("Parse = %v, %v; want %v", got, err, want)
	}
	if _, err := Parse("app.toml", data); err == nil {
		t.Error("Parse(app.toml) should fail on the name, as Load does")
	}
}
