// Package render writes a drift report for people (text) or machines (JSON).
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/eduardofrafre/confdrift/internal/drift"
)

// Options controls how values are shown.
type Options struct {
	ShowSecrets bool
	Color       bool
}

const (
	red    = "\x1b[31m"
	yellow = "\x1b[33m"
	dim    = "\x1b[2m"
	bold   = "\x1b[1m"
	reset  = "\x1b[0m"
)

// Text writes one block per drifted key followed by a summary line. Files are
// labelled by the part of their path that tells them apart.
func Text(w io.Writer, r drift.Report, opts Options) error {
	paint := func(code, s string) string {
		if !opts.Color {
			return s
		}
		return code + s + reset
	}
	names := ShortNames(r.Envs)
	width := 0
	for _, n := range names {
		width = max(width, len(n))
	}

	var b strings.Builder
	for _, d := range r.Diffs {
		label := paint(yellow, "changed")
		if d.Kind == drift.Missing {
			var absent []string
			for i, v := range d.Values {
				if v == nil {
					absent = append(absent, names[i])
				}
			}
			label = paint(red, "missing from "+strings.Join(absent, ", "))
		}
		fmt.Fprintf(&b, "%s  %s\n", paint(bold, d.Key), label)
		for i, v := range d.Values {
			shown := paint(dim, "(not set)")
			if v != nil {
				shown = value(d.Key, *v, opts)
			}
			fmt.Fprintf(&b, "  %-*s  %s\n", width, names[i], shown)
		}
		b.WriteByte('\n')
	}
	b.WriteString(summary(r))
	b.WriteByte('\n')
	_, err := io.WriteString(w, b.String())
	return err
}

func summary(r drift.Report) string {
	files := fmt.Sprintf("%d %s", len(r.Envs), plural(len(r.Envs), "file"))
	if len(r.Diffs) == 0 {
		return fmt.Sprintf("No drift: %d %s match across %s.", r.Keys, plural(r.Keys, "key"), files)
	}
	var parts []string
	if n := r.Count(drift.Missing); n > 0 {
		parts = append(parts, fmt.Sprintf("%d missing", n))
	}
	if n := r.Count(drift.Changed); n > 0 {
		parts = append(parts, fmt.Sprintf("%d changed", n))
	}
	return fmt.Sprintf("%d of %d %s drifted across %s (%s).",
		len(r.Diffs), r.Keys, plural(r.Keys, "key"), files, strings.Join(parts, ", "))
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func value(key, v string, opts Options) string {
	if opts.ShowSecrets {
		return v
	}
	return drift.Redact(key, v)
}

type jsonReport struct {
	Files   []string   `json:"files"`
	Keys    int        `json:"keys"`
	Drifted int        `json:"drifted"`
	Drift   []jsonDiff `json:"drift"`
}

type jsonDiff struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	// Values holds the files that define the key, with typed JSON values.
	Values map[string]json.RawMessage `json:"values"`
	// Missing lists the files that do not define it.
	Missing []string `json:"missing,omitempty"`
}

// JSON writes the report as one JSON object.
func JSON(w io.Writer, r drift.Report, opts Options) error {
	out := jsonReport{Files: r.Envs, Keys: r.Keys, Drifted: len(r.Diffs), Drift: []jsonDiff{}}
	for _, d := range r.Diffs {
		jd := jsonDiff{Key: d.Key, Kind: string(d.Kind), Values: map[string]json.RawMessage{}}
		for i, v := range d.Values {
			if v == nil {
				jd.Missing = append(jd.Missing, r.Envs[i])
				continue
			}
			jd.Values[r.Envs[i]] = rawJSON(value(d.Key, *v, opts))
		}
		out.Drift = append(out.Drift, jd)
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// rawJSON passes canonical values through as typed JSON and quotes anything
// that is not valid JSON on its own, such as the redaction marker.
func rawJSON(s string) json.RawMessage {
	if json.Valid([]byte(s)) {
		return json.RawMessage(s)
	}
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s) // a string always encodes
	return json.RawMessage(strings.TrimSuffix(b.String(), "\n"))
}
