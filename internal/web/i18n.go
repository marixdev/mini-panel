package web

import "fmt"

// vi is the Vietnamese catalog and en the English one. Each area of the panel has one file per language,
// i18n_vi_<area>.go and i18n_en_<area>.go, holding the same keys in the same order. HTTP handlers and templates use a
// request-local translator; T keeps Vietnamese as the default for non-HTTP callers. Missing translations fall back to
// Vietnamese, then to the key itself.
// Text that comes from the mpn commands themselves (the output of an operation) is not translated.
var (
	vi = map[string]string{}
	en = map[string]string{}
)

// T returns the text for key, filled in with args when there are any.
func T(key string, args ...any) string {
	s, ok := vi[key]
	if !ok {
		return key
	}
	return formatText(s, args...)
}

func formatText(s string, args ...any) string {
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}
