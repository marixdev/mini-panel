package web

import "fmt"

// vi is the Vietnamese catalog, en the English one and zh the Simplified Chinese one. Each area of the panel has one
// file per language, i18n_vi_<area>.go, i18n_en_<area>.go and i18n_zh_<area>.go, holding the same keys in the same
// order. HTTP handlers and templates use a request-local translator; T keeps Vietnamese as the default for non-HTTP
// callers. Missing translations fall back to Vietnamese, then to the key itself.
// Text that comes from the mpn commands themselves (the output of an operation) is not translated.
var (
	vi = map[string]string{}
	en = map[string]string{}
	zh = map[string]string{}
)

// catalogs maps a language code to its catalog; Vietnamese is the default and is not listed.
var catalogs = map[string]map[string]string{"en": en, "zh": zh}

// language is one language of the switcher: its code, the short label of narrow screens and its own name.
type language struct{ Code, Short, Name string }

// languages are the languages of the panel, in the order the switcher shows them.
var languages = []language{
	{"vi", "VI", "Tiếng Việt"},
	{"en", "EN", "English"},
	{"zh", "中文", "简体中文"},
}

func supportedLanguage(code string) bool {
	for _, l := range languages {
		if l.Code == code {
			return true
		}
	}
	return false
}

func languageOf(code string) language {
	for _, l := range languages {
		if l.Code == code {
			return l
		}
	}
	return languages[0]
}

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
