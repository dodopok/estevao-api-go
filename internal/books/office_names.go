package books

import (
	"github.com/dodopok/estevao-api-go/internal/i18n"
	"github.com/dodopok/estevao-api-go/internal/langs"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

const officeNamesFallbackLocale = "pt-BR"

// OfficeName ports PrayerBooks::OfficeNames.for.
func OfficeName(office, language string, familyRite bool) string {
	form := "standard"
	if familyRite {
		form = "family"
	}
	locale := officeNamesLocale(form, language)
	if i18n.Available(locale) {
		if v, ok := i18n.T(locale, "daily_office.office_names."+form+"."+office); ok {
			return v
		}
	}
	return rb.Humanize(office)
}

func officeNamesLocale(form, language string) string {
	if rb.BlankString(language) {
		return officeNamesFallbackLocale
	}
	locale := langs.OfficeNamesLocaleFor(language)
	if locale != "" && i18n.Available(locale) && i18n.Exists(locale, "daily_office.office_names."+form) {
		return locale
	}
	return officeNamesFallbackLocale
}

// OfficeNames ports PrayerBook#office_names.
func (c *Capabilities) OfficeNames(language string, familyRite bool) []any {
	form := "standard"
	if familyRite {
		form = "family"
	}
	overrides, _ := c.DailyOffice().Dig("labels", form).(*rb.Map)
	out := []any{}
	for _, key := range c.AvailableOffices(familyRite) {
		var label any
		if overrides != nil {
			if v := overrides.Get(key); rb.Present(v) {
				label = v
			}
		}
		m := rb.M("key", key)
		if label != nil {
			m.Set("name", label)
			m.Set("book_label", label)
		} else {
			m.Set("name", OfficeName(key, language, familyRite))
		}
		out = append(out, m)
	}
	return out
}
