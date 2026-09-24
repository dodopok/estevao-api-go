// Package i18n loads the Rails locale files (config/locales, copied here)
// and resolves keys with the fallback chain Rails uses in production
// (config.i18n.fallbacks = true, default locale :en).
package i18n

import (
	"embed"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yml
var files embed.FS

var (
	once  sync.Once
	store = map[string]map[string]any{}
)

// activeSupportEN holds the parts of ActiveSupport's own en.yml the
// application reads.
var activeSupportEN = map[string]any{
	"date": map[string]any{
		"month_names":      []any{nil, "January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
		"abbr_month_names": []any{nil, "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
		"day_names":        []any{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
	},
}

func load() {
	store["en"] = map[string]any{}
	deepMerge(store["en"], activeSupportEN)
	entries, _ := files.ReadDir("locales")
	for _, e := range entries {
		b, err := files.ReadFile("locales/" + e.Name())
		if err != nil {
			continue
		}
		var doc map[string]any
		if err := yaml.Unmarshal(b, &doc); err != nil {
			panic("i18n: " + e.Name() + ": " + err.Error())
		}
		for locale, tree := range doc {
			m, ok := tree.(map[string]any)
			if !ok {
				continue
			}
			if store[locale] == nil {
				store[locale] = map[string]any{}
			}
			deepMerge(store[locale], m)
		}
	}
}

func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			dm, ok := dst[k].(map[string]any)
			if !ok {
				dm = map[string]any{}
				dst[k] = dm
			}
			deepMerge(dm, sm)
			continue
		}
		dst[k] = v
	}
}

// Fallbacks returns the I18n fallback chain for a locale.
func Fallbacks(locale string) []string {
	chain := []string{locale}
	if i := strings.Index(locale, "-"); i > 0 {
		chain = append(chain, locale[:i])
	}
	if locale != "en" {
		chain = append(chain, "en")
	}
	return chain
}

func lookup(locale, key string) (any, bool) {
	var cur any = store[locale]
	for _, part := range strings.Split(key, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// Lookup resolves key in locale with fallbacks.
func Lookup(locale, key string) (any, bool) {
	once.Do(load)
	for _, l := range Fallbacks(locale) {
		// A nil entry (YAML ~) counts as missing, as in the I18n backend.
		if v, ok := lookup(l, key); ok && v != nil {
			return v, true
		}
	}
	return nil, false
}

// T returns the string at key ("" and false when missing).
func T(locale, key string) (string, bool) {
	v, ok := Lookup(locale, key)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// Exists ports I18n.exists?(key, locale) (with fallbacks).
func Exists(locale, key string) bool {
	_, ok := Lookup(locale, key)
	return ok
}

// Available reports whether any file defines the locale.
func Available(locale string) bool {
	once.Do(load)
	_, ok := store[locale]
	return ok
}
