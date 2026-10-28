// internal/generate/gocode_internal_test.go

package generate

import (
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/message"
)

func plainMsg(key string) *message.Message {
	return &message.Message{Key: key, Kind: message.KindPlain, Value: "x"}
}

func withParam(key string) *message.Message {
	m := plainMsg(key)
	m.Placeholders = map[string]message.Placeholder{"name": {Name: "name", Type: message.TypeString}}
	return m
}

// Two keys that emit one Go identifier would make loc report success for a package that does not compile.
func TestEmittedIdentifiersNeverCollide(t *testing.T) {
	cases := map[string][]*message.Message{
		"accessor vs key constant":     {plainMsg("x.y"), plainMsg("key.x.y")},
		"accessor vs DefaultLocale":    {plainMsg("default.locale")},
		"accessor vs Locales":          {plainMsg("locales")},
		"accessor vs NewBundle":        {plainMsg("new.bundle")},
		"accessor vs LocaleFiles":      {plainMsg("locale.files")},
		"accessor vs another's Params": {withParam("a"), plainMsg("a.params")},
	}
	for name, msgs := range cases {
		if _, err := funcNames(msgs); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := funcNames([]*message.Message{plainMsg("x.y"), withParam("a")}); err != nil {
		t.Fatalf("distinct keys refused: %v", err)
	}
}

// A description is free text; a line break in it must not become live code.
func TestADescriptionStaysOneCommentLine(t *testing.T) {
	m := plainMsg("greeting")
	m.Description = "the greeting\nfunc Evil() {}\n// more"
	out := accessor(m, "Greeting")
	if strings.Count(out, "\nfunc ") != 1 || strings.Contains(out, "\nfunc Evil") {
		t.Fatalf("description escaped its comment:\n%s", out)
	}
}

// go doc shows the comment above the function; a Params type gets its own line.
func TestTheDocCommentSitsOnTheAccessor(t *testing.T) {
	m := withParam("greeting")
	m.Description = "renders the greeting"
	out := accessor(m, "Greeting")
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "func Greeting(") {
			if lines[i-1] != "// Greeting renders the greeting" {
				t.Fatalf("line above the accessor is %q", lines[i-1])
			}
			return
		}
	}
	t.Fatal("no accessor")
}
