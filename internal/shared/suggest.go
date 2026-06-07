// internal/shared/suggest.go

package shared

import (
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
)

// vocabulary is the controlled shared namespace, keyed on the lowercased
// default-locale text.
//
// Deliberately short: it is what stops a suggestion inventing a key, and
// `common` as an open list is how a shared key ends up meaning four things.
var vocabulary = map[string]string{
	"ok":        "common.action.accept",
	"accept":    "common.action.accept",
	"confirm":   "common.action.accept",
	"apply":     "common.action.accept",
	"save":      "common.action.save",
	"cancel":    "common.action.cancel",
	"dismiss":   "common.action.cancel",
	"close":     "common.action.close",
	"retry":     "common.action.retry",
	"try again": "common.action.retry",
	"delete":    "common.action.delete",
	"remove":    "common.action.delete",
	"edit":      "common.action.edit",
	"yes":       "common.answer.yes",
	"no":        "common.answer.no",
	"loading":   "common.state.loading",
	"loading…":  "common.state.loading",
	"back":      "common.navigation.back",
	"next":      "common.navigation.next",
	"previous":  "common.navigation.previous",
}

// Suggest proposes a shared key, or returns empty when it cannot.
//
// Only answers for text in the controlled vocabulary, and only for a plain
// string with no placeholders: an interpolated or counted message carries
// context a generic name would lose.
func Suggest(cfg *config.Config, set *message.Set, group []string) string {
	base, ok := set.Get(cfg.DefaultLocale, group[0])
	if !ok {
		return ""
	}
	if base.Kind != message.KindPlain || len(base.Placeholders) > 0 {
		return ""
	}

	// If one key is already shared, point the others at it.
	if existing := alreadyShared(group); existing != "" {
		return existing
	}

	name, ok := vocabulary[normalizeLookup(base.Value)]
	if !ok {
		return ""
	}

	// A suggested key that already exists has its own contract, so none is offered.
	if _, taken := set.Get(cfg.DefaultLocale, name); taken {
		return ""
	}

	// Already in the shared namespace, so there is nothing to promote.
	for _, key := range group {
		if key == name {
			return ""
		}
	}
	return name
}

// alreadyShared returns the group's sole member in the shared namespace, or empty.
func alreadyShared(group []string) string {
	var found string
	for _, key := range group {
		if !strings.HasPrefix(key, sharedNamespace+".") {
			continue
		}
		if found != "" {
			return ""
		}
		found = key
	}
	return found
}

const sharedNamespace = "common"

func normalizeLookup(text string) string {
	clean := strings.TrimSpace(strings.ToLower(text))
	return strings.TrimRight(clean, ".!…")
}
