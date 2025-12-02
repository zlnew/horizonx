// Package pkg
package pkg

import "strings"

func ContainsAny(s string, xs []string) bool {
	s = strings.ToLower(s)

	for _, x := range xs {
		x = strings.ToLower(x)

		if strings.HasSuffix(x, "*") {
			prefix := strings.TrimSuffix(x, "*")
			if strings.HasPrefix(s, prefix) {
				return true
			}
			continue
		}

		if strings.Contains(s, x) {
			return true
		}
	}

	return false
}
