// Package drift compares flattened configurations from several environments
// and reports every key that is missing somewhere or whose value differs.
package drift

import (
	"regexp"
	"sort"
	"strings"

	"github.com/eduardofrafre/confdrift/internal/config"
)

// Kind classifies a drifted key.
type Kind string

const (
	// Missing: at least one environment does not define the key.
	Missing Kind = "missing"
	// Changed: every environment defines the key, with different values.
	Changed Kind = "changed"
)

// Env is one named configuration taking part in the comparison.
type Env struct {
	Name   string
	Values config.Values
}

// Diff is one drifted key. Values is aligned with Report.Envs; a nil entry
// means that environment does not define the key.
type Diff struct {
	Key    string
	Kind   Kind
	Values []*string
}

// Report is the outcome of a comparison.
type Report struct {
	Envs  []string
	Keys  int // distinct keys compared, after ignores
	Diffs []Diff
}

// Options narrows what counts as drift.
type Options struct {
	// Ignore holds key patterns to leave out; * matches any run of characters.
	Ignore []string
	// KeysOnly reports missing keys and skips value differences, for
	// environments whose values are expected to differ.
	KeysOnly bool
}

// Compare checks every key across all environments.
func Compare(envs []Env, opts Options) Report {
	ignore := compileGlobs(opts.Ignore)
	keys := map[string]struct{}{}
	for _, e := range envs {
		for k := range e.Values {
			if ignore == nil || !ignore.MatchString(k) {
				keys[k] = struct{}{}
			}
		}
	}

	r := Report{Keys: len(keys)}
	for _, e := range envs {
		r.Envs = append(r.Envs, e.Name)
	}
	for k := range keys {
		d := Diff{Key: k, Values: make([]*string, len(envs))}
		missing, changed := false, false
		var first *string
		for i, e := range envs {
			v, ok := e.Values[k]
			if !ok {
				missing = true
				continue
			}
			d.Values[i] = &v
			if first == nil {
				first = &v
			} else if v != *first {
				changed = true
			}
		}
		switch {
		case missing:
			d.Kind = Missing
		case changed && !opts.KeysOnly:
			d.Kind = Changed
		default:
			continue
		}
		r.Diffs = append(r.Diffs, d)
	}
	sort.Slice(r.Diffs, func(i, j int) bool { return r.Diffs[i].Key < r.Diffs[j].Key })
	return r
}

// Count returns how many diffs are of the given kind.
func (r Report) Count(k Kind) int {
	n := 0
	for _, d := range r.Diffs {
		if d.Kind == k {
			n++
		}
	}
	return n
}

// compileGlobs turns patterns into one anchored regexp, or nil when there are
// none.
func compileGlobs(patterns []string) *regexp.Regexp {
	if len(patterns) == 0 {
		return nil
	}
	alts := make([]string, len(patterns))
	for i, p := range patterns {
		alts[i] = strings.ReplaceAll(regexp.QuoteMeta(p), `\*`, `.*`)
	}
	return regexp.MustCompile(`^(?:` + strings.Join(alts, "|") + `)$`)
}

// Redacted replaces a value that must not be printed.
const Redacted = "<redacted>"

var (
	secretKey = regexp.MustCompile(`(?i)(secret|passw|pwd|token|api[_.-]?key|private[_.-]?key|credential|auth|dsn|cert)`)
	// scheme://user:password@host, as in DATABASE_URL or REDIS_URL
	urlPassword = regexp.MustCompile(`(://[^:/@\s"]*:)[^@/\s"]+@`)
)

// Redact hides a value that may hold a credential: the whole value when the
// key name suggests a secret, otherwise only a password embedded in a URL.
// Values are compared before redaction, so hidden drift is still reported.
func Redact(key, value string) string {
	if secretKey.MatchString(key) {
		return Redacted
	}
	return urlPassword.ReplaceAllString(value, "${1}"+Redacted+"@")
}
