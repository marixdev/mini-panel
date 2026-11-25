package update

import (
	"strings"
	"testing"
)

const changelog = `# Changelog

Intro text.

## [Unreleased]

- work in progress

## [1.4.0] - 2026-12-01

- not released yet

## [1.3.0] - 2026-11-25

### Added

- **Remote access** to databases.

## [1.2.3] - 2026-11-22

### Fixed

- A fix.

## [1.2.2] - 2026-11-19

- Older.
`

func TestTheChangelogIsCutIntoReleases(t *testing.T) {
	s := Sections(changelog, Version{1, 3, 0})
	if len(s) != 3 || s[0].Version != "1.3.0" || s[0].Date != "2026-11-25" || s[2].Version != "1.2.2" {
		t.Fatalf("%+v", s)
	}
	if !strings.HasPrefix(s[0].Body, "### Added") || !strings.Contains(s[0].Body, "**Remote access**") || strings.Contains(s[0].Body, "A fix") {
		t.Errorf("body of 1.3.0: %q", s[0].Body)
	}
	n := Newer(s, Version{1, 2, 2})
	if len(n) != 2 || n[0].Version != "1.3.0" || n[1].Version != "1.2.3" {
		t.Errorf("newer than 1.2.2: %+v", n)
	}
	if len(Newer(s, Version{1, 3, 0})) != 0 {
		t.Error("nothing is newer than the latest")
	}
}

func TestEachLanguageHasItsFile(t *testing.T) {
	v := Version{1, 3, 0}
	for lang, want := range map[string]string{"en": "/v1.3.0/CHANGELOG.md", "vi": "/v1.3.0/CHANGELOG.vi.md", "zh": "/v1.3.0/CHANGELOG.zh.md", "fr": "/v1.3.0/CHANGELOG.md"} {
		if u := ChangelogURL(v, lang); !strings.HasSuffix(u, want) || !strings.HasPrefix(u, "https://raw.githubusercontent.com/"+Repo+"/") {
			t.Errorf("%s: %s", lang, u)
		}
	}
}
