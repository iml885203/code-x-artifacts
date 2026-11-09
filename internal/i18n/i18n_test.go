package i18n

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func load(t *testing.T) *Catalogues {
	t.Helper()
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAcceptLanguagePicksTraditionalChineseForAnyZhAndEnglishForTheRest(t *testing.T) {
	c := load(t)
	for header, want := range map[string]string{
		"zh-TW":                         "zh-Hant",
		"zh-CN":                         "zh-Hant",
		"zh":                            "zh-Hant",
		"zh-Hant-HK":                    "zh-Hant",
		"ZH-tw":                         "zh-Hant",
		"en-US":                         "en",
		"en":                            "en",
		"ja":                            "en",
		"fr":                            "en",
		"":                              "en",
		"*":                             "en",
		"zh-TW,zh;q=0.9,en;q=0.8":       "zh-Hant",
		"en-US,en;q=0.9,zh-TW;q=0.8":    "en",
		"en;q=0.5, zh-CN;q=0.9":         "zh-Hant",
		"ja,zh;q=0.5":                   "zh-Hant", // ja is not served, so the next best is
		"ja,fr;q=0.5":                   "en",
		"zh;q=0,en":                     "en",
		"zh;q=0":                        "en",
		"en;q=0.8,zh;q=0.8":             "en", // a tie goes to the one listed first
		"zh;q=0.8,en;q=0.8":             "zh-Hant",
		"zh;q=banana,en;q=0.5":          "en", // an unreadable weight counts as unwanted
		"  zh-TW ; q=0.7 , en ; q=0.6 ": "zh-Hant",
		"zh-TW;q=0.3,en;q=0.2,fr;q=0.9": "zh-Hant",
	} {
		if got := c.Pick(header).Tag; got != want {
			t.Errorf("Accept-Language %q: got %s, want %s", header, got, want)
		}
	}
}

func TestEveryLanguageHasExactlyTheKeysOfEnglish(t *testing.T) {
	c := load(t)
	english := c.Get(Fallback).Keys()
	for _, tag := range c.Tags() {
		l := c.Get(tag)
		keys := l.Keys()
		for _, k := range english {
			if !slices.Contains(keys, k) {
				t.Errorf("%s lacks %q", tag, k)
			}
		}
		for _, k := range keys {
			if !slices.Contains(english, k) {
				t.Errorf("%s has %q, which English lacks", tag, k)
			}
		}
		for _, k := range keys {
			if strings.TrimSpace(l.T(k)) == "" {
				t.Errorf("%s has an empty text for %q", tag, k)
			}
		}
	}
}

func TestEveryLanguageHasExactlyTheAreaFilesOfEnglish(t *testing.T) {
	c := load(t)
	english := c.Get(Fallback).Areas()
	if len(english) < 5 {
		t.Fatalf("found only %d areas: the loader is broken", len(english))
	}
	for _, tag := range c.Tags() {
		if got := c.Get(tag).Areas(); !slices.Equal(got, english) {
			t.Errorf("%s has the areas %v, English has %v", tag, got, english)
		}
	}
}

func TestAKeyOutsideItsAreaFileFailsTheLoad(t *testing.T) {
	file := func(json string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(json)} }
	_, err := loadFrom(fstest.MapFS{
		"locales/en/nav.json":      file(`{"nav.home": "Home", "gallery.title": "Artifacts"}`),
		"locales/zh-Hant/nav.json": file(`{"nav.home": "首頁"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "gallery.title") {
		t.Fatalf("got %v", err)
	}
}

func TestTheAreasAreMergedIntoOneCatalogue(t *testing.T) {
	file := func(json string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(json)} }
	c, err := loadFrom(fstest.MapFS{
		"locales/en/nav.json":      file(`{"nav.home": "Home"}`),
		"locales/en/card.json":     file(`{"card.pin": "Pin"}`),
		"locales/zh-Hant/nav.json": file(`{"nav.home": "首頁"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if l := c.Get("en"); l.T("nav.home") != "Home" || l.T("card.pin") != "Pin" || !slices.Equal(l.Areas(), []string{"card", "nav"}) {
		t.Errorf("got %v in areas %v", l.Keys(), l.Areas())
	}
}

func TestEveryLanguageTheBuildServesHasACatalogue(t *testing.T) {
	c := load(t)
	for tag := range matches {
		if c.Get(tag) == nil {
			t.Errorf("%s is served but has no catalogue", tag)
		}
	}
	for _, tag := range c.Tags() {
		if _, ok := matches[tag]; !ok {
			t.Errorf("%s has a catalogue but no Accept-Language match", tag)
		}
	}
}

func TestPrefixedHandsOutEntriesWithoutThePrefixAndEmptyMeansEverything(t *testing.T) {
	l := load(t).Get("en")
	got := l.Prefixed("lang.")
	if got["en"] != "EN" || got["zh-Hant"] != "中" || got["label"] == "" || len(got) != 3 {
		t.Fatalf("got %v", got)
	}
	if all := l.Prefixed(""); len(all) != len(l.Keys()) {
		t.Errorf("the empty prefix gave %d of %d entries", len(all), len(l.Keys()))
	}
}

func TestAPickedLanguageWinsOverAcceptLanguageAndAnUnknownOneIsIgnored(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ picked, accept, want string }{
		{"en", "zh-TW", "en"},
		{"zh-Hant", "en-US", "zh-Hant"},
		{"zh-Hant", "", "zh-Hant"},
		{"", "zh-TW", "zh-Hant"},
		{"", "en-US", "en"},
		{"", "", "en"},
		{"ja", "zh-TW", "zh-Hant"},
		{"zh", "zh-TW", "zh-Hant"},
		{"EN", "zh-TW", "zh-Hant"},
	} {
		if got := c.Choose(tc.picked, tc.accept).Tag; got != tc.want {
			t.Errorf("Choose(%q, %q) = %s, want %s", tc.picked, tc.accept, got, tc.want)
		}
	}
}
