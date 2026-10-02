// Command confdrift compares configuration files across environments and
// exits non-zero when they drift apart.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/eduardofrafre/confdrift/internal/config"
	"github.com/eduardofrafre/confdrift/internal/drift"
	"github.com/eduardofrafre/confdrift/internal/render"
)

// Exit codes follow diff(1): 0 no drift, 1 drift found, 2 trouble.
const (
	exitClean = 0
	exitDrift = 1
	exitError = 2
)

// version is set by the release build with -ldflags "-X main.version=...".
// A go install build reports the module version instead.
var version = "dev"

const usage = `Usage: confdrift [flags] FILE FILE [FILE...]

Compares configuration files from different environments and reports keys
that are missing from some of them or hold different values.

Formats are picked from the file name: .env (also .env.production, prod.env),
.yaml/.yml and .json. Nested keys are flattened to paths like db.pool.max.
Kubernetes ConfigMaps are compared by their data keys.

Exit status is 0 when the files agree, 1 when they drift, 2 on error.

Flags:
`

type ignoreList []string

func (l *ignoreList) String() string     { return strings.Join(*l, ",") }
func (l *ignoreList) Set(v string) error { *l = append(*l, v); return nil }

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, isTerminal(os.Stdout)))
}

func run(args []string, stdout, stderr io.Writer, tty bool) int {
	fs := flag.NewFlagSet("confdrift", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		ignore      ignoreList
		keysOnly    = fs.Bool("keys-only", false, "report only missing keys, not value differences")
		format      = fs.String("format", "text", "output format: text or json")
		showSecrets = fs.Bool("show-secrets", false, "print values of keys that look like credentials")
		showVersion = fs.Bool("version", false, "print the version and exit")
	)
	fs.Var(&ignore, "ignore", "skip keys matching `PATTERN` (* is a wildcard); repeatable")
	fs.Usage = func() {
		fmt.Fprint(stderr, usage)
		fs.PrintDefaults()
	}

	files, err := parseInterspersed(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		return exitClean
	}
	if err != nil {
		return exitError
	}
	if *showVersion {
		v := version
		if info, ok := debug.ReadBuildInfo(); ok && v == "dev" && info.Main.Version != "(devel)" && info.Main.Version != "" {
			v = info.Main.Version
		}
		fmt.Fprintln(stdout, "confdrift", v)
		return exitClean
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "confdrift: unknown format %q (use text or json)\n", *format)
		return exitError
	}
	if len(files) < 2 {
		fs.Usage()
		return exitError
	}

	envs := make([]drift.Env, len(files))
	for i, f := range files {
		vals, err := config.Load(f)
		if err != nil {
			fmt.Fprintln(stderr, "confdrift:", err)
			return exitError
		}
		envs[i] = drift.Env{Name: f, Values: vals}
	}

	report := drift.Compare(envs, drift.Options{Ignore: ignore, KeysOnly: *keysOnly})
	opts := render.Options{ShowSecrets: *showSecrets, Color: tty && os.Getenv("NO_COLOR") == ""}
	if *format == "json" {
		err = render.JSON(stdout, report, opts)
	} else {
		err = render.Text(stdout, report, opts)
	}
	if err != nil {
		fmt.Fprintln(stderr, "confdrift:", err)
		return exitError
	}
	if len(report.Diffs) > 0 {
		return exitDrift
	}
	return exitClean
}

// parseInterspersed lets flags appear after file names
// (confdrift dev.env prod.env --keys-only), which the flag package alone stops
// parsing at. Everything after "--" is a file name.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var files []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			return files, nil
		}
		// flag stops at "--" and drops it; detect that by the arg before rest.
		if consumed := len(args) - len(rest); consumed > 0 && args[consumed-1] == "--" {
			return append(files, rest...), nil
		}
		files = append(files, rest[0])
		args = rest[1:]
	}
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
