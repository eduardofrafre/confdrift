//go:build js && wasm

// Command wasm is the confdrift engine compiled for the browser, so the site's
// playground runs the same parsing, comparison and rendering as the CLI.
//
// It registers one function, confdrift(request), where request is a JSON
// string {"files":[{"name","text"}],"keysOnly","ignore":[],"showSecrets"} and
// the result is a JSON string {"exit","text","json","error"}.
package main

import (
	"encoding/json"
	"strings"
	"syscall/js"

	"github.com/eduardofrafre/confdrift/internal/config"
	"github.com/eduardofrafre/confdrift/internal/drift"
	"github.com/eduardofrafre/confdrift/internal/render"
)

type request struct {
	Files []struct {
		Name string `json:"name"`
		Text string `json:"text"`
	} `json:"files"`
	KeysOnly    bool     `json:"keysOnly"`
	Ignore      []string `json:"ignore"`
	ShowSecrets bool     `json:"showSecrets"`
}

type response struct {
	Exit  int    `json:"exit"`
	Text  string `json:"text,omitempty"`
	JSON  string `json:"json,omitempty"`
	Error string `json:"error,omitempty"`
}

func run(raw string) response {
	var req request
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		return response{Exit: 2, Error: err.Error()}
	}
	envs := make([]drift.Env, len(req.Files))
	for i, f := range req.Files {
		vals, err := config.Parse(f.Name, []byte(f.Text))
		if err != nil {
			return response{Exit: 2, Error: "confdrift: " + err.Error()}
		}
		envs[i] = drift.Env{Name: f.Name, Values: vals}
	}
	report := drift.Compare(envs, drift.Options{Ignore: req.Ignore, KeysOnly: req.KeysOnly})

	var text, out strings.Builder
	render.Text(&text, report, render.Options{ShowSecrets: req.ShowSecrets, Color: true})
	render.JSON(&out, report, render.Options{ShowSecrets: req.ShowSecrets})
	exit := 0
	if len(report.Diffs) > 0 {
		exit = 1
	}
	return response{Exit: exit, Text: text.String(), JSON: out.String()}
}

func main() {
	js.Global().Set("confdrift", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return `{"exit":2,"error":"no request"}`
		}
		b, _ := json.Marshal(run(args[0].String()))
		return string(b)
	}))
	select {}
}
