package config

import (
	"fmt"
	"strings"
)

// parseEnv reads dotenv syntax: KEY=value, an optional "export " prefix,
// # comments, single quotes taken literally, double quotes with escapes and
// line breaks. Variables are not expanded: ${HOST} drifting is drift.
// A repeated key keeps its last value, as dotenv loaders do.
func parseEnv(data []byte) (Values, error) {
	vals := Values{}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		lineNo := i + 1
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, rest, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.ContainsAny(key, " \t") {
			return nil, fmt.Errorf("line %d: expected KEY=value", lineNo)
		}
		rest = strings.TrimLeft(rest, " \t")

		var value string
		switch {
		case strings.HasPrefix(rest, `"`):
			// A double-quoted value may continue on the following lines.
			body := rest[1:]
			for {
				v, after, closed := readDoubleQuoted(body)
				if closed {
					if err := trailing(after, lineNo); err != nil {
						return nil, err
					}
					value = v
					break
				}
				i++
				if i == len(lines) {
					return nil, fmt.Errorf("line %d: unterminated double quote", lineNo)
				}
				body += "\n" + lines[i]
			}
		case strings.HasPrefix(rest, "'"):
			end := strings.IndexByte(rest[1:], '\'')
			if end < 0 {
				return nil, fmt.Errorf("line %d: unterminated single quote", lineNo)
			}
			if err := trailing(rest[end+2:], lineNo); err != nil {
				return nil, err
			}
			value = rest[1 : end+1]
		default:
			// An unquoted value ends at " #", so a # inside a URL survives.
			if j := strings.Index(rest, " #"); j >= 0 {
				rest = rest[:j]
			}
			value = strings.TrimSpace(rest)
		}
		vals[key] = canonical(value)
	}
	return vals, nil
}

// readDoubleQuoted scans up to the closing quote, resolving escapes. It
// reports whether the quote was closed and returns whatever follows it.
func readDoubleQuoted(s string) (value, after string, closed bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"':
			return b.String(), s[i+1:], true
		case c == '\\' && i+1 < len(s):
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(s[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", "", false
}

// trailing allows only whitespace or a comment after a closing quote.
func trailing(s string, lineNo int) error {
	s = strings.TrimSpace(s)
	if s != "" && !strings.HasPrefix(s, "#") {
		return fmt.Errorf("line %d: unexpected text after closing quote", lineNo)
	}
	return nil
}
