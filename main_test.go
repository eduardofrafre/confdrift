package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fixtures(t *testing.T) (dev, prod string) {
	dir := t.TempDir()
	dev = write(t, dir, "dev.env", "LOG_LEVEL=debug\nCACHE_TTL=300\nDB_PASSWORD=devpass\nPORT=8080\n")
	prod = write(t, dir, "prod.env", "LOG_LEVEL=info\nDB_PASSWORD=prodpass\nPORT=8080\n")
	return dev, prod
}

func TestRunText(t *testing.T) {
	dev, prod := fixtures(t)
	var out, errOut bytes.Buffer
	code := run([]string{dev, prod}, &out, &errOut, false)
	if code != exitDrift {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	s := out.String()
	for _, want := range []string{
		"CACHE_TTL  missing from prod.env",
		"LOG_LEVEL  changed",
		`"debug"`,
		"(not set)",
		"DB_PASSWORD  changed",
		"<redacted>",
		"3 of 4 keys drifted across 2 files (1 missing, 2 changed).",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "devpass") || strings.Contains(s, "prodpass") {
		t.Errorf("secret printed:\n%s", s)
	}
	if strings.Contains(s, "\x1b[") {
		t.Error("color written to a non-terminal")
	}
}

func TestRunFlagsAfterFiles(t *testing.T) {
	dev, prod := fixtures(t)
	var out bytes.Buffer
	code := run([]string{dev, prod, "--keys-only", "--show-secrets"}, &out, &bytes.Buffer{}, false)
	if code != exitDrift {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(out.String(), "LOG_LEVEL") || !strings.Contains(out.String(), "CACHE_TTL") {
		t.Errorf("--keys-only ignored:\n%s", out.String())
	}
}

func TestRunJSON(t *testing.T) {
	dev, prod := fixtures(t)
	var out bytes.Buffer
	code := run([]string{"--format", "json", "--ignore", "LOG_*", dev, prod}, &out, &bytes.Buffer{}, false)
	if code != exitDrift {
		t.Fatalf("exit %d", code)
	}
	var got struct {
		Keys    int `json:"keys"`
		Drifted int `json:"drifted"`
		Drift   []struct {
			Key     string                     `json:"key"`
			Kind    string                     `json:"kind"`
			Values  map[string]json.RawMessage `json:"values"`
			Missing []string                   `json:"missing"`
		} `json:"drift"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if got.Keys != 3 || got.Drifted != 2 {
		t.Errorf("keys=%d drifted=%d, want 3 and 2", got.Keys, got.Drifted)
	}
	cache := got.Drift[0]
	if cache.Key != "CACHE_TTL" || string(cache.Values[dev]) != `"300"` || len(cache.Missing) != 1 || cache.Missing[0] != prod {
		t.Errorf("CACHE_TTL entry = %+v", cache)
	}
	if string(got.Drift[1].Values[dev]) != `"<redacted>"` {
		t.Errorf("secret not redacted in JSON: %s", got.Drift[1].Values[dev])
	}
}

func TestRunClean(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.yaml", "db:\n  port: 5432\n")
	b := write(t, dir, "b.json", `{"db": {"port": 5432}}`)
	var out bytes.Buffer
	if code := run([]string{a, b}, &out, &bytes.Buffer{}, false); code != exitClean {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "No drift: 1 key match across 2 files.") {
		t.Errorf("got %q", out.String())
	}
}

func TestRunErrors(t *testing.T) {
	dev, _ := fixtures(t)
	for name, args := range map[string][]string{
		"one file":       {dev},
		"missing file":   {dev, "nope.env"},
		"unknown format": {dev, dev, "--format", "xml"},
		"unknown flag":   {"--bogus", dev, dev},
		"unknown type":   {dev, "config.toml"},
	} {
		var errOut bytes.Buffer
		if code := run(args, &bytes.Buffer{}, &errOut, false); code != exitError {
			t.Errorf("%s: exit %d, want %d", name, code, exitError)
		}
		if errOut.Len() == 0 {
			t.Errorf("%s: nothing on stderr", name)
		}
	}
}

func TestRunDoubleDash(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.env", "A=1\n")
	write(t, dir, "-b.env", "A=1\n")
	t.Chdir(dir)
	if code := run([]string{a, "--", "-b.env"}, &bytes.Buffer{}, &bytes.Buffer{}, false); code != exitClean {
		t.Errorf("file after -- not treated as a file: exit %d", code)
	}
}
