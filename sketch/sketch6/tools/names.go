package tools

import "strings"

func JoinNames(defs []Definition) string {
	parts := make([]string, 0, len(defs))
	for _, def := range defs {
		parts = append(parts, def.Name)
	}
	return JoinNamesFromStrings(parts)
}

func JoinNamesFromStrings(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}
