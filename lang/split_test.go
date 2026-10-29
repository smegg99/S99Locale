// lang/split_test.go

package lang

import "testing"

// split reduces a locale to its language and reports C, C.UTF-8 and POSIX.
func TestSplitReadsTheLanguage(t *testing.T) {
	for _, c := range []struct {
		in  string
		tag string
		isC bool
	}{
		{"pl_PL.UTF-8", "pl", false},
		{"pl@euro", "pl", false},
		{"pl-PL", "pl", false},
		{"PL", "pl", false},
		{"  pl  ", "pl", false},
		{"C", "", true},
		{"c", "", true},
		{"POSIX", "", true},
		{"C.UTF-8", "", true},
		{"C.utf8", "", true},
		{"", "", false},
	} {
		tag, _, isC := split(c.in)
		if tag != c.tag || isC != c.isC {
			t.Errorf("split(%q) = (%q, %v), want (%q, %v)", c.in, tag, isC, c.tag, c.isC)
		}
	}
}
