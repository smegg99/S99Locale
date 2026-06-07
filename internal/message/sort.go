// internal/message/sort.go

package message

import "sort"

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SortedPlaceholders returns placeholders sorted by name, for stable generated output.
func SortedPlaceholders(in map[string]Placeholder) []Placeholder {
	out := make([]Placeholder, 0, len(in))
	for _, p := range in {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
