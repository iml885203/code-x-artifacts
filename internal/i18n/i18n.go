// Package i18n holds the interface's translations and picks one per request.
//
// Each language is a directory in locales/, named by its language tag, with one
// flat JSON file per area (locales/<tag>/<area>.json). An area is a key
// namespace: the file nav.json holds exactly the keys "nav.*", so a feature adds
// its text to its own file, and the server merges the files at startup.
// The English catalogue is the reference: every other language must have
// exactly its areas and keys (TestEveryLanguageHasExactlyTheKeysOfEnglish).
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed locales/*/*.json
var localeFS embed.FS

// Fallback is served when nothing the browser asks for is available.
const Fallback = "en"

// Lang is one language's catalogue.
type Lang struct {
	// Tag is the BCP 47 tag, used for <html lang> and Content-Language.
	Tag  string
	text map[string]string
	// areas is the key namespace of every file the language was loaded from.
	areas map[string]bool
}

// Areas lists the language's areas (the names of its files) in order.
func (l *Lang) Areas() []string {
	out := make([]string, 0, len(l.areas))
	for a := range l.areas {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// T returns the text for key. A missing key is a programming error that the
// catalogue tests catch before release; at runtime it shows as the key itself
// so the page still renders.
func (l *Lang) T(key string) string {
	if s, ok := l.text[key]; ok {
		return s
	}
	return key
}

// Prefixed returns every entry whose key starts with prefix, keyed by the
// rest of the key. The empty prefix is the whole catalogue, which the shell
// hands the app as a JSON block.
func (l *Lang) Prefixed(prefix string) map[string]string {
	out := map[string]string{}
	for k, v := range l.text {
		if rest, ok := strings.CutPrefix(k, prefix); ok {
			out[rest] = v
		}
	}
	return out
}

// Keys lists the catalogue's keys in order.
func (l *Lang) Keys() []string {
	keys := make([]string, 0, len(l.text))
	for k := range l.text {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Catalogues is every language this build has.
type Catalogues struct {
	langs map[string]*Lang
	// match maps a primary language subtag ("zh") to the language that serves it.
	match map[string]string
}

// matches says which primary language subtags each language serves. Adding a
// language means adding its file and one line here.
var matches = map[string][]string{
	"en":      {"en"},
	"zh-Hant": {"zh"},
}

// Load reads every embedded catalogue.
func Load() (*Catalogues, error) { return loadFrom(localeFS) }

func loadFrom(localeFS fs.FS) (*Catalogues, error) {
	c := &Catalogues{langs: map[string]*Lang{}, match: map[string]string{}}
	files, err := fs.Glob(localeFS, "locales/*/*.json")
	if err != nil {
		return nil, fmt.Errorf("list locales: %w", err)
	}
	for _, f := range files {
		tag, area, _ := strings.Cut(strings.TrimPrefix(f, "locales/"), "/")
		area = strings.TrimSuffix(area, ".json")
		l := c.langs[tag]
		if l == nil {
			l = &Lang{Tag: tag, text: map[string]string{}, areas: map[string]bool{}}
			c.langs[tag] = l
		}
		raw, err := fs.ReadFile(localeFS, f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var entries map[string]string
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		for k, v := range entries {
			if !strings.HasPrefix(k, area+".") {
				return nil, fmt.Errorf("%s has %q, which is not in the %q area", f, k, area)
			}
			l.text[k] = v
		}
		l.areas[area] = true
	}
	for tag, subtags := range matches {
		if _, ok := c.langs[tag]; !ok {
			return nil, fmt.Errorf("language %s has no catalogue", tag)
		}
		for _, sub := range subtags {
			c.match[sub] = tag
		}
	}
	if _, ok := c.langs[Fallback]; !ok {
		return nil, fmt.Errorf("fallback language %s has no catalogue", Fallback)
	}
	return c, nil
}

// Get returns the catalogue for a tag, or nil.
func (c *Catalogues) Get(tag string) *Lang { return c.langs[tag] }

// Tags lists the available language tags in order.
func (c *Catalogues) Tags() []string {
	tags := make([]string, 0, len(c.langs))
	for t := range c.langs {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

// Choose is the language for a request: the one the person picked (the value of
// their language cookie) when this build has it, otherwise whatever Pick makes
// of Accept-Language. An unknown or stale cookie value is ignored, so removing
// a language never strands anyone.
func (c *Catalogues) Choose(picked, acceptLanguage string) *Lang {
	if l, ok := c.langs[picked]; ok {
		return l
	}
	return c.Pick(acceptLanguage)
}

// Pick chooses the language for an Accept-Language header: the most preferred
// language (by q-value, then by position) that this build serves. Any zh
// variant is Traditional Chinese. Nothing usable, or no header, is English.
func (c *Catalogues) Pick(acceptLanguage string) *Lang {
	type want struct {
		sub string
		q   float64
		pos int
	}
	var wants []want
	for pos, part := range strings.Split(acceptLanguage, ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		tag = strings.TrimSpace(tag)
		if tag == "" || tag == "*" {
			continue
		}
		q := 1.0
		for _, p := range strings.Split(params, ";") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(p), "q="); ok {
				f, err := strconv.ParseFloat(v, 64)
				if err != nil || f < 0 || f > 1 {
					f = 0
				}
				q = f
			}
		}
		sub, _, _ := strings.Cut(strings.ToLower(tag), "-")
		wants = append(wants, want{sub, q, pos})
	}
	sort.SliceStable(wants, func(i, j int) bool { return wants[i].q > wants[j].q })
	for _, w := range wants {
		if w.q == 0 {
			break
		}
		if tag, ok := c.match[w.sub]; ok {
			return c.langs[tag]
		}
	}
	return c.langs[Fallback]
}
