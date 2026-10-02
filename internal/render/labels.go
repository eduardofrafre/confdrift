package render

import (
	"path/filepath"
	"strings"
)

// ShortNames drops the path parts every file shares, so
// overlays/staging/configmap.yaml and overlays/production/configmap.yaml read
// as staging and production.
func ShortNames(paths []string) []string {
	parts := make([][]string, len(paths))
	for i, p := range paths {
		parts[i] = strings.Split(filepath.ToSlash(filepath.Clean(p)), "/")
	}
	// all reports whether every path keeps more than one part and agrees on
	// the part that at picks.
	all := func(at func([]string) string) bool {
		for _, p := range parts {
			if len(p) < 2 || at(p) != at(parts[0]) {
				return false
			}
		}
		return true
	}
	for len(parts) > 1 && all(func(p []string) string { return p[0] }) {
		for i := range parts {
			parts[i] = parts[i][1:]
		}
	}
	if len(parts) > 1 && all(func(p []string) string { return p[len(p)-1] }) {
		for i := range parts {
			parts[i] = parts[i][:len(parts[i])-1]
		}
	}
	names := make([]string, len(parts))
	for i, p := range parts {
		names[i] = strings.Join(p, "/")
	}
	return names
}
