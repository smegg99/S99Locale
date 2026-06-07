// internal/pseudo/pseudo.go

// Package pseudo generates a fake locale that is still readable.
//
// It exists to make three bugs visible before a translator ever sees the app:
// a string that never went through i18n stays plain ASCII, a layout that only
// fits English overflows, and a font missing glyphs shows boxes.
//
// Everything here is deterministic. A pseudolocale that changed between builds
// would break the byte-identical guarantee the rest of the tool relies on.
package pseudo

import (
	"strings"

	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/plural"
)

// Locale is the code the generated pseudolocale is written under.
//
// Not a real language tag: it must never be selected by accident, and it is
// exempt from the cross-locale completeness checks because it is complete by
// construction.
const Locale = "pseudo"

// accents maps ASCII to a look-alike. Chosen to stay readable - the point is
// to spot untranslated text, not to make the app unusable.
var accents = map[rune]rune{
	'a': 'á', 'b': 'ƀ', 'c': 'ç', 'd': 'đ', 'e': 'é', 'f': 'ƒ', 'g': 'ğ',
	'h': 'ĥ', 'i': 'ï', 'j': 'ĵ', 'k': 'ķ', 'l': 'ł', 'm': 'ɱ', 'n': 'ñ',
	'o': 'ö', 'p': 'þ', 'q': 'ɋ', 'r': 'ř', 's': 'š', 't': 'ŧ', 'u': 'ü',
	'v': 'ṽ', 'w': 'ŵ', 'x': 'ẋ', 'y': 'ý', 'z': 'ž',
	'A': 'Á', 'B': 'Ɓ', 'C': 'Ç', 'D': 'Đ', 'E': 'É', 'F': 'Ƒ', 'G': 'Ğ',
	'H': 'Ĥ', 'I': 'Ï', 'J': 'Ĵ', 'K': 'Ķ', 'L': 'Ł', 'M': 'Ṁ', 'N': 'Ñ',
	'O': 'Ö', 'P': 'Þ', 'Q': 'Ɋ', 'R': 'Ř', 'S': 'Š', 'T': 'Ŧ', 'U': 'Ü',
	'V': 'Ṽ', 'W': 'Ŵ', 'X': 'Ẋ', 'Y': 'Ý', 'Z': 'Ž',
}

// padding is how much longer the pseudolocale is than the source.
//
// German and Polish routinely run 30% longer than English, so a layout that
// only fits the original is a layout that will break. The brackets make a
// truncated string obvious: a missing ] is a clipped label.
const padding = 0.3

// Text pseudolocalises one string, leaving placeholders untouched: a mangled
// {name} would stop interpolating and show a bug this tool invented.
func Text(in string) string {
	if in == "" {
		return ""
	}

	var b strings.Builder
	b.WriteRune('[')

	runes := []rune(in)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '{' {
			b.WriteRune(accent(runes[i]))
			continue
		}

		end := indexRune(runes[i:], '}')
		if end < 0 {
			b.WriteRune(accent(runes[i]))
			continue
		}
		// Verbatim, braces included.
		b.WriteString(string(runes[i : i+end+1]))
		i += end
	}

	b.WriteString(strings.Repeat("·", pad(len(runes))))
	b.WriteRune(']')
	return b.String()
}

func accent(r rune) rune {
	if to, ok := accents[r]; ok {
		return to
	}
	return r
}

// pad is deterministic in the source length, never random.
func pad(length int) int {
	n := int(float64(length) * padding)
	if n < 1 {
		return 1
	}
	return n
}

func indexRune(runes []rune, want rune) int {
	for i, r := range runes {
		if r == want {
			return i
		}
	}
	return -1
}

// Message builds the pseudolocale version of a message.
//
// The plural bands come from the pseudolocale's own rule, not the source's:
// generating four Polish bands for a locale with two would produce a message
// no index function can address.
func Message(m *message.Message) (*message.Message, error) {
	out := *m
	out.Locale = Locale

	if m.Kind == message.KindPlain {
		out.Value = Text(m.Value)
		return &out, nil
	}

	rule, err := plural.For(Locale)
	if err != nil {
		return nil, err
	}

	forms := map[message.Category]string{}
	for _, c := range rule.Required {
		// Source the text from the matching band when there is one, and from
		// "other" when there is not.
		text, ok := m.Plural.Forms[c]
		if !ok {
			text = m.Plural.Forms[message.CatOther]
		}
		forms[c] = Text(text)
	}
	if zero, ok := m.Plural.Forms[message.CatZero]; ok {
		forms[message.CatZero] = Text(zero)
	}

	out.Plural = &message.Plural{Argument: m.Plural.Argument, Forms: forms}
	return &out, nil
}
