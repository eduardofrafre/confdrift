// Package config reads configuration files of different formats into one flat
// shape: a map from key path to canonical value. Everything downstream compares
// those maps and never sees the original format.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Values maps a flattened key path (db.host, servers[0], DATABASE_URL) to its
// canonical value: the JSON encoding of the scalar, so the string "5432" and
// the number 5432 stay distinguishable.
type Values map[string]string

// Format is a supported input format.
type Format string

const (
	Env  Format = "env"
	YAML Format = "yaml"
	JSON Format = "json"
)

// Detect picks a format from the file name: .json, .yaml/.yml, or any of the
// dotenv spellings (.env, .env.production, prod.env).
func Detect(path string) (Format, error) {
	base := filepath.Base(path)
	switch strings.ToLower(filepath.Ext(base)) {
	case ".json":
		return JSON, nil
	case ".yaml", ".yml":
		return YAML, nil
	case ".env":
		return Env, nil
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return Env, nil
	}
	return "", fmt.Errorf("%s: cannot tell the format from the file name (use .env, .yaml, .yml or .json)", path)
}

// Load reads and flattens one file.
func Load(path string) (Values, error) {
	format, err := Detect(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var vals Values
	switch format {
	case Env:
		vals, err = parseEnv(data)
	case YAML:
		vals, err = parseYAML(data)
	case JSON:
		vals, err = parseJSON(data)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return vals, nil
}

// canonical encodes a scalar the same way regardless of source format. HTML
// escaping is off so a value like a&b prints as written, not a\u0026b.
func canonical(v any) string {
	// Fast paths for the common scalars, byte-identical to the encoder.
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case json.Number:
		return string(t)
	case string:
		if plainASCII(t) {
			return `"` + t + `"`
		}
	}
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprint(v) // NaN and Inf have no JSON form
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// plainASCII reports whether s is printable ASCII that JSON quotes as is.
func plainASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c > 0x7e || c == '"' || c == '\\' {
			return false
		}
	}
	return true
}
