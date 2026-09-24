// Package config reads the process environment. The Rails application reads
// ENV at call time in many places, so this is a thin accessor rather than a
// struct loaded once: tests may change values between requests.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Get returns the variable or "".
func Get(key string) string { return os.Getenv(key) }

// Present reports whether the variable is set to a non-blank value.
func Present(key string) bool { return strings.TrimSpace(os.Getenv(key)) != "" }

// Fetch returns the variable or def when unset (ENV.fetch semantics: an
// empty string is returned as-is when set).
func Fetch(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

// PresenceOr returns the variable when present, else def.
func PresenceOr(key, def string) string {
	if Present(key) {
		return os.Getenv(key)
	}
	return def
}

// PositiveInt returns ENV[key].presence&.to_i when positive, else def.
func PositiveInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n := rubyToI(v)
	if n > 0 {
		return n
	}
	return def
}

func rubyToI(s string) int {
	i := 0
	neg := false
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		neg = s[i] == '-'
		i++
	}
	n := 0
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		return -n
	}
	return n
}

// Bool casts like ActiveModel::Type::Boolean (nil for unset/empty).
func Bool(key string) (value bool, set bool) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return false, false
	}
	switch v {
	case "0", "f", "F", "false", "FALSE", "off", "OFF":
		return false, true
	}
	return true, true
}

// Int parses an integer env var with a default.
func Int(key string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil {
		return v
	}
	return def
}

// Production is always true for the Go service: it reproduces the Rails
// production behaviour (error rendering, verification defaults).
const Production = true

// RailsEnv is Rails.env (RAILS_ENV, "development" when unset).
func RailsEnv() string { return PresenceOr("RAILS_ENV", "development") }

// PublicDir is the directory Rails serves as public/ (the local audio files
// live under public/audio). PUBLIC_DIR overrides the default "public".
func PublicDir() string { return PresenceOr("PUBLIC_DIR", "public") }
