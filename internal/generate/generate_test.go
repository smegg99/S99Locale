// internal/generate/generate_test.go

package generate_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/parse"
)

const cfgBody = `
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web", "backend"]
roots: ["locales"]
namespaces: {
	common: {targets: ["web"]}
	emails: {targets: ["backend"]}
}
go: {package: "messages"}
`

// project writes a fixture and builds it, without touching disk for output.
func project(t *testing.T, files map[string]string) (*config.Config, *generate.Outputs) {
	t.Helper()
	dir := t.TempDir()

	for name, body := range files {
		full := filepath.Join(dir, "locales", name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfgPath := filepath.Join(dir, "config.cue")
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(context.Background(), cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	res, err := parse.Roots(context.Background(), cfg.AbsRoots(), cfg.Locales)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Errors {
		t.Fatalf("fixture does not parse: %v", e)
	}

	out, err := generate.Build(context.Background(), cfg, res.Set)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, out
}

// file finds a staged file by target and path.
func file(t *testing.T, outs *generate.Outputs, want string) []byte {
	t.Helper()

	var have []string
	for _, target := range []message.Target{message.TargetBackend, message.TargetWeb} {
		out := outs.For(target)
		if out == nil {
			continue
		}
		for _, f := range out.Files() {
			full := string(target) + "/" + f.Path
			if full == want {
				return f.Bytes
			}
			have = append(have, full)
		}
	}
	t.Fatalf("no %s; have %v", want, have)
	return nil
}

func jsonFile(t *testing.T, out *generate.Outputs, path string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(file(t, out, path), &v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return v
}

var fixture = map[string]string{
	"en/common.yaml": `
cancel:
  value: "Cancel"
greeting:
  value: "Hello {name}"
  placeholders:
    name: {type: string}
`,
	"pl/common.yaml": `
cancel:
  value: "Anuluj"
greeting:
  value: "Cześć {name}"
  placeholders:
    name: {type: string}
`,
	"en/emails.yaml": `
welcome:
  value: "Welcome, {name}"
  description: "Sent after signup"
  placeholders:
    name: {type: string}
`,
	"pl/emails.yaml": `
welcome:
  value: "Witaj, {name}"
  placeholders:
    name: {type: string}
`,
}

// Target filtering: web must not carry backend-only namespaces, and vice versa.
func TestTargetFiltering(t *testing.T) {
	_, out := project(t, fixture)

	web := jsonFile(t, out, "web/en/common.json")
	if _, ok := web["common"]; !ok {
		t.Fatalf("web bundle = %v", web)
	}
	for _, f := range out.For(message.TargetWeb).Files() {
		if f.Path == "en/emails.json" {
			t.Fatal("a backend-only namespace reached the web bundle")
		}
	}

	backend := jsonFile(t, out, "backend/locales/en.json")
	if _, ok := backend["emails.welcome"]; !ok {
		t.Fatalf("backend bundle = %v", backend)
	}
	if _, ok := backend["common.cancel"]; ok {
		t.Fatal("a web-only namespace reached the backend bundle")
	}
}

// vue-i18n keeps {name}; go-i18n needs {{.Name}}. Same canonical message.
func TestOneMessageTwoRuntimeForms(t *testing.T) {
	_, out := project(t, fixture)

	web := jsonFile(t, out, "web/en/common.json")
	common := web["common"].(map[string]any)
	if got := common["greeting"]; got != "Hello {name}" {
		t.Fatalf("vue form = %v", got)
	}

	backend := jsonFile(t, out, "backend/locales/en.json")
	if got := backend["emails.welcome"]; got != "Welcome, {{.Name}}" {
		t.Fatalf("go form = %v", got)
	}
}

func TestVueBundleIsNested(t *testing.T) {
	_, out := project(t, map[string]string{
		"en/common.yaml": "action:\n  accept:\n    value: \"OK\"\n",
		"pl/common.yaml": "action:\n  accept:\n    value: \"OK\"\n",
	})

	web := jsonFile(t, out, "web/en/common.json")
	common, ok := web["common"].(map[string]any)
	if !ok {
		t.Fatalf("bundle = %v", web)
	}
	action, ok := common["action"].(map[string]any)
	if !ok || action["accept"] != "OK" {
		t.Fatalf("not nested: %v", common)
	}
}
