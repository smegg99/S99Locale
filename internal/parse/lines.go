// internal/parse/lines.go

package parse

import (
	"bufio"
	"bytes"
	"strings"

	"github.com/smegg99/s99locale/internal/message"
)

// MaxFileSize is the largest locale file the line index will scan. Beyond it
// bufio.Scanner fails with "token too long", which names neither the file nor
// the reason, so parse checks the size first and says so itself.
const MaxFileSize = 4 << 20

// lineFinder locates a key in the raw bytes, by its dotted path within the
// file.
//
// The YAML decoder gives a plain map, so positions have to come from the text.
// Keyed on the whole path rather than the leaf name because `title` appears
// under every section of a real locale file, and a diagnostic that points at
// the first one sends the reader to the wrong line with no hint that it did.
type lineFinder map[string][2]int

// open is a key the scanner is currently inside, with the column it started at.
type open struct {
	name   string
	indent int
}

func lineIndex(raw []byte) lineFinder {
	out := lineFinder{}
	var stack []open

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), MaxFileSize)

	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()

		trimmed := strings.TrimLeft(text, " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		name, ok := strings.CutSuffix(keyPart(trimmed), ":")
		if !ok || name == "" {
			continue
		}
		name = strings.Trim(name, `"'`)

		// Indentation at or left of the current key closes it.
		indent := len(text) - len(trimmed)
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, open{name: name, indent: indent})

		path := joinPath(stack)
		if _, seen := out[path]; !seen {
			out[path] = [2]int{line, indent + 1}
		}
	}
	return out
}

func joinPath(stack []open) string {
	var b strings.Builder
	for i, o := range stack {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(o.name)
	}
	return b.String()
}

// keyPart takes everything up to and including the first colon.
func keyPart(line string) string {
	if i := strings.Index(line, ":"); i >= 0 {
		return line[:i+1]
	}
	return line
}

// of resolves a path within the file; an unknown path resolves to the file, not another line.
func (f lineFinder) of(path, file string) message.Source {
	if pos, ok := f[path]; ok {
		return message.Source{File: file, Line: pos[0], Col: pos[1]}
	}
	return message.Source{File: file}
}
