package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func parseJSON(data []byte) (Values, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // keep 9007199254740993 exact instead of rounding through float64
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after the JSON document")
	}
	vals := Values{}
	flatten(vals, "", doc)
	return vals, nil
}

// parseYAML flattens a single YAML document. Kubernetes ConfigMaps are the
// exception: only their data and binaryData keys count, and a file holding
// several manifests (helm template output, a kustomize build) contributes the
// keys of all its ConfigMaps, the same way envFrom merges them in a pod.
func parseYAML(data []byte) (Values, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var docs []any
	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		keepTimestampsAsText(&node)
		var doc any
		if err := node.Decode(&doc); err != nil {
			return nil, err
		}
		if doc != nil {
			docs = append(docs, doc)
		}
	}

	vals := Values{}
	configMaps := 0
	for _, doc := range docs {
		m, ok := doc.(map[string]any)
		if !ok || m["kind"] != "ConfigMap" {
			continue
		}
		configMaps++
		for _, field := range []string{"data", "binaryData"} {
			entries, _ := m[field].(map[string]any)
			for k, v := range entries {
				if _, dup := vals[k]; dup {
					return nil, fmt.Errorf("key %q is defined by more than one ConfigMap", k)
				}
				vals[k] = canonical(v)
			}
		}
	}
	if configMaps > 0 {
		return vals, nil
	}

	switch len(docs) {
	case 0:
		return vals, nil
	case 1:
		flatten(vals, "", docs[0])
		return vals, nil
	default:
		return nil, fmt.Errorf("%d YAML documents and none is a ConfigMap; split them into one file each", len(docs))
	}
}

// keepTimestampsAsText retags dates as strings. Decoded as timestamps,
// 2024-01-01 would print as 2024-01-01T00:00:00Z and differ from the same date
// in a JSON or .env file.
func keepTimestampsAsText(n *yaml.Node) {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!timestamp" {
		n.Tag = "!!str"
	}
	for _, c := range n.Content {
		keepTimestampsAsText(c)
	}
}

// flatten walks nested maps and lists, writing one entry per leaf. Empty maps
// and lists are leaves too, so "features: {}" against a missing key is drift.
func flatten(out Values, prefix string, v any) {
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 0 && prefix != "" {
			out[prefix] = "{}"
		}
		for k, child := range t {
			flatten(out, joinKey(prefix, k), child)
		}
	case map[any]any: // YAML allows non-string keys such as 1 or true
		if len(t) == 0 && prefix != "" {
			out[prefix] = "{}"
		}
		for k, child := range t {
			flatten(out, joinKey(prefix, fmt.Sprint(k)), child)
		}
	case []any:
		if len(t) == 0 && prefix != "" {
			out[prefix] = "[]"
		}
		for i, child := range t {
			flatten(out, prefix+"["+strconv.Itoa(i)+"]", child)
		}
	default:
		if prefix != "" {
			out[prefix] = canonical(t)
		}
	}
}

// joinKey appends a segment with a dot, or in brackets when the segment would
// make the path ambiguous: labels["app.kubernetes.io/name"].
func joinKey(prefix, key string) string {
	if key == "" || strings.ContainsAny(key, ".[]\" ") {
		return prefix + "[" + strconv.Quote(key) + "]"
	}
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}
