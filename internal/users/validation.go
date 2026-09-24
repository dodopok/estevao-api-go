package users

import (
	"regexp"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// AvailableVoices is LiturgicalText::AVAILABLE_VOICES.
var AvailableVoices = []string{"male_1", "female_1", "male_2"}

// ValidVoice reports whether v is one of AvailableVoices.
func ValidVoice(v any) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	for _, x := range AvailableVoices {
		if x == s {
			return true
		}
	}
	return false
}

var twoLetters = regexp.MustCompile(`^[A-Z]{2}$`)
var countrySep = regexp.MustCompile(`[-_]`)

// NormalizeCountryCode ports the before_validation normalize_country_code.
func NormalizeCountryCode(code *string) *string {
	if code == nil || rb.BlankString(*code) {
		return code
	}
	parts := countrySep.Split(*code, -1)
	last := strings.ToUpper(parts[len(parts)-1])
	if rb.BlankString(last) {
		return nil
	}
	return &last
}

// Errors is ActiveModel::Errors#full_messages for a users row, in the
// order the model declares its validations. The uniqueness validations only
// query when their attribute changed, which none of the ported updates do.
func (u *User) Errors() []string {
	var out []string
	if rb.BlankString(u.Email) {
		out = append(out, "Email can't be blank")
	}
	if u.ProviderUID == nil || rb.BlankString(*u.ProviderUID) {
		out = append(out, "Provider uid can't be blank")
	}
	if rb.BlankString(u.Timezone) {
		out = append(out, "Timezone can't be blank")
	} else if rb.RailsZone(u.Timezone) == nil {
		out = append(out, "Timezone is not a valid timezone (e.g., 'America/Sao_Paulo', 'Europe/London')")
	}
	if u.CountryCode != nil && !rb.BlankString(*u.CountryCode) && !twoLetters.MatchString(*u.CountryCode) {
		out = append(out, "Country code must be a two-letter country code")
	}
	if u.Preferences != nil {
		if v := u.Preferences.Get("preferred_audio_voice"); !rb.Blank(v) && !ValidVoice(v) {
			out = append(out, "Preferences preferred_audio_voice must be one of: "+strings.Join(AvailableVoices, ", "))
		}
	}
	return out
}
