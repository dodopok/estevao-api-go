package wrapped

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	countryOnce   sync.Once
	countryByZone map[string][]string
)

func zoneinfoDir() string {
	if d := os.Getenv("TZDIR"); d != "" {
		return d
	}
	for _, d := range []string{"/usr/share/zoneinfo", "/usr/share/lib/zoneinfo", "/etc/zoneinfo"} {
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			return d
		}
	}
	return ""
}

// loadCountries ports TZInfo's ZoneinfoDataSource country index: the
// countries of iso3166.tab with their zones from zone1970.tab (zone.tab
// when it is absent), read from the system zoneinfo directory.
func loadCountries() {
	countryByZone = map[string][]string{}
	dir := zoneinfoDir()
	if dir == "" {
		return
	}
	known := map[string]bool{}
	if f, err := os.Open(filepath.Join(dir, "iso3166.tab")); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
				continue
			}
			known[strings.SplitN(line, "\t", 2)[0]] = true
		}
		f.Close()
	}
	tab := filepath.Join(dir, "zone1970.tab")
	if _, err := os.Stat(tab); err != nil {
		tab = filepath.Join(dir, "zone.tab")
	}
	f, err := os.Open(tab)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 3 {
			continue
		}
		for _, code := range strings.Split(cols[0], ",") {
			if known[code] {
				countryByZone[cols[2]] = append(countryByZone[cols[2]], code)
			}
		}
	}
}

// CountryFromTimezone ports CountryFromTimezone.call: the country of a
// timezone that belongs to exactly one country ("" otherwise). Symlinked
// zone names resolve to their canonical zone first.
func CountryFromTimezone(tz string) string {
	countryOnce.Do(loadCountries)
	if strings.TrimSpace(tz) == "" {
		return ""
	}
	canonical := tz
	if dir := zoneinfoDir(); dir != "" {
		p := filepath.Join(dir, tz)
		if _, err := os.Stat(p); err == nil {
			if real, err := filepath.EvalSymlinks(p); err == nil {
				canonical = strings.TrimPrefix(real, dir+"/")
			}
		}
	}
	if c := countryByZone[canonical]; len(c) == 1 {
		return c[0]
	}
	return ""
}
