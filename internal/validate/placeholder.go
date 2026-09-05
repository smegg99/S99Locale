// internal/validate/placeholder.go

package validate

import (
	"strings"
	"unicode"
)

// Placeholders extracts {name} occurrences in order, skipping vue-i18n's {{ }} literal braces.
func Placeholders(text string) []string {
	var out []string

	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		if i+1 < len(text) && text[i+1] == '{' {
			// Literal brace. Skip both, and the closing pair if present.
			i++
			continue
		}

		end := strings.IndexByte(text[i:], '}')
		if end < 0 {
			break
		}
		name := text[i+1 : i+end]
		if valid(name) {
			out = append(out, name)
		}
		i += end
	}
	return out
}

// valid keeps the extractor from treating prose in braces as a placeholder.
func valid(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case unicode.IsLetter(r), r == '_':
		case unicode.IsDigit(r) && i > 0:
		default:
			return false
		}
	}
	return true
}

// GoField is the Go template field a placeholder becomes, email -> Email.
func GoField(name string) string {
	if name == "" {
		return ""
	}

	var b strings.Builder
	upper := true
	for _, r := range name {
		if r == '_' || r == '-' {
			upper = true
			continue
		}
		if upper {
			b.WriteRune(unicode.ToUpper(r))
			upper = false
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
