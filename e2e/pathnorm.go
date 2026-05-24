//go:build integration

package e2e

import "strings"

// NormalizeLeafPath strips Cobra placeholder tokens (e.g. <iid>) for comparison with CommandCase.Path.
func NormalizeLeafPath(p string) string {
	var parts []string
	for _, w := range strings.Fields(p) {
		if strings.HasPrefix(w, "<") {
			continue
		}
		parts = append(parts, w)
	}
	return strings.Join(parts, " ")
}
