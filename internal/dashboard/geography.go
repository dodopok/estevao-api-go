package dashboard

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// defaultTimezone ports User::DEFAULT_TIMEZONE.
const defaultTimezone = "America/Sao_Paulo"

var (
	zoneCountriesOnce sync.Once
	zoneinfoDir       string
	zoneCountries     map[string][]string
	zoneTabLine       = regexp.MustCompile(`\A([A-Z]{2}(?:,[A-Z]{2})*)\t[+\-0-9]+\t([^\t]+)`)
	iso3166Line       = regexp.MustCompile(`\A([A-Z]{2})(?:\t[A-Z]{3}\t[0-9]{3})?\t(.+)\z`)
)

// loadZoneCountries ports CountryFromTimezone::COUNTRY_CODES_BY_ZONE from
// the system zoneinfo directory, as TZInfo's ZoneinfoDataSource reads it:
// zone1970.tab (else zone.tab) lists each zone's countries, restricted to
// the countries iso3166.tab names.
func loadZoneCountries() {
	zoneCountries = map[string][]string{}
	for _, dir := range []string{"/usr/share/zoneinfo", "/usr/share/lib/zoneinfo", "/etc/zoneinfo"} {
		iso := filepath.Join(dir, "iso3166.tab")
		tab := filepath.Join(dir, "zone1970.tab")
		if _, err := os.Stat(tab); err != nil {
			tab = filepath.Join(dir, "zone.tab")
		}
		if _, err := os.Stat(iso); err != nil {
			continue
		}
		if _, err := os.Stat(tab); err != nil {
			continue
		}
		zoneinfoDir = dir
		known := map[string]bool{}
		readLines(iso, func(line string) {
			if m := iso3166Line.FindStringSubmatch(line); m != nil {
				known[m[1]] = true
			}
		})
		readLines(tab, func(line string) {
			m := zoneTabLine.FindStringSubmatch(line)
			if m == nil {
				return
			}
			for _, code := range strings.Split(m[1], ",") {
				if known[code] && !contains(zoneCountries[m[2]], code) {
					zoneCountries[m[2]] = append(zoneCountries[m[2]], code)
				}
			}
		})
		return
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func readLines(path string, f func(string)) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		f(strings.TrimRight(sc.Text(), "\r"))
	}
}

// CountryFromTimezone ports CountryFromTimezone.call: the one country of
// the canonical zone (its real path under the zoneinfo directory), or nil.
func CountryFromTimezone(timezone string) any {
	if rb.BlankString(timezone) {
		return nil
	}
	zoneCountriesOnce.Do(loadZoneCountries)
	canonical := timezone
	if zoneinfoDir != "" {
		path := filepath.Join(zoneinfoDir, timezone)
		if _, err := os.Stat(path); err == nil {
			if real, err := filepath.EvalSymlinks(path); err == nil {
				if rootReal, err := filepath.EvalSymlinks(zoneinfoDir); err == nil {
					canonical = strings.TrimPrefix(real, rootReal+"/")
				}
			}
		}
	}
	if c := zoneCountries[canonical]; len(c) == 1 {
		return c[0]
	}
	return nil
}

// Geography ports Dashboard::GeographyMetrics.
func (p *Period) Geography() *rb.Map {
	type row struct {
		country            any
		explicit, inferred bool
		language           string
	}
	var list []row
	total, explicitUsers, defaultTZ := 0, 0, 0
	rows := p.rows(`SELECT "users"."country_code", "users"."timezone", "users"."preferences"::text FROM "users"`)
	for rows.Next() {
		var cc, tz, prefs *string
		must(rows.Scan(&cc, &tz, &prefs))
		total++
		if cc != nil && !rb.BlankString(*cc) {
			explicitUsers++
		}
		if tz != nil && *tz == defaultTimezone {
			defaultTZ++
		}
		r := row{language: "unknown"}
		if cc != nil && !rb.BlankString(*cc) {
			r.country, r.explicit = strings.ToUpper(*cc), true
		} else if c := CountryFromTimezone(rb.ToS(rb.Deref(tz))); c != nil {
			r.country, r.inferred = c, true
		}
		if prefs != nil {
			if parsed, err := rb.ParseJSON([]byte(*prefs)); err == nil {
				if m, ok := parsed.(*rb.Map); ok {
					if v := m.Get("language"); v != nil && v != false {
						r.language = hashKey(v)
					}
				}
			}
		}
		list = append(list, r)
	}
	rows.Close()
	must(rows.Err())
	resolved, inferred, unresolved := 0, 0, 0
	byCountry, byLanguage := rb.NewMap(), rb.NewMap()
	type breakdown struct {
		code                        string
		users, explicit, tzInferred int
	}
	var codes []string
	bd := map[string]*breakdown{}
	for _, r := range list {
		if r.country != nil {
			resolved++
		} else {
			unresolved++
		}
		if r.inferred {
			inferred++
		}
		key := "other"
		if r.country != nil {
			key = rb.ToS(r.country)
		}
		inc(byCountry, key)
		inc(byLanguage, r.language)
		e, ok := bd[key]
		if !ok {
			e = &breakdown{code: key}
			bd[key] = e
			codes = append(codes, key)
		}
		e.users++
		if r.explicit {
			e.explicit++
		}
		if r.inferred {
			e.tzInferred++
		}
	}
	var entries []*breakdown
	for _, c := range codes {
		entries = append(entries, bd[c])
	}
	sortStable(entries, func(a, b *breakdown) bool {
		return a.users > b.users || a.users == b.users && a.code < b.code
	})
	countryBreakdown := []any{}
	for _, e := range entries {
		countryBreakdown = append(countryBreakdown, rb.M("users", e.users, "explicit_users", e.explicit,
			"timezone_inferred_users", e.tzInferred, "country_code", e.code))
	}
	ft := float64(total)
	return scopePayload("lifetime", rb.M(
		"total_users", total, "explicit_country_users", explicitUsers,
		"country_coverage_percentage", percentage(float64(explicitUsers), ft),
		"default_timezone_users", defaultTZ, "inferred_country_users", inferred,
		"resolved_country_users", resolved, "derived_country_users", resolved,
		"ambiguous_or_unknown_timezone_users", unresolved,
		"coverage", rb.M("total_users", total, "explicit_country_users", explicitUsers,
			"timezone_inferred_country_users", inferred, "resolved_country_users", resolved,
			"unresolved_country_users", unresolved,
			"explicit_country_percentage", percentage(float64(explicitUsers), ft),
			"resolved_country_percentage", percentage(float64(resolved), ft)),
		"by_country", byCountry, "country_breakdown", countryBreakdown, "by_language", byLanguage))
}
