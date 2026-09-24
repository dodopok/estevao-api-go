package liturgical

import (
	"github.com/dodopok/estevao-api-go/internal/i18n"
	"github.com/dodopok/estevao-api-go/internal/langs"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// PresentFastObservance ports Liturgical::FastObservancePresenter#call.
func PresentFastObservance(f *FastObservance, language string) any {
	if f == nil {
		return nil
	}
	locale := langs.ExplanationLocaleFor(language)
	if locale == "" {
		locale = "pt-BR"
	}
	t := func(group, key string) string {
		s, ok := i18n.T(locale, "liturgical.fast_observances."+group+"."+key)
		if !ok {
			panic("translation missing: " + locale + ".liturgical.fast_observances." + group + "." + key)
		}
		return s
	}
	return rb.M(
		"kind", f.Kind, "status", f.Status, "reason", f.Reason,
		"explanation", rb.M(
			"badge_label", t("badge_labels", f.Kind),
			"title", t("kinds", f.Kind),
			"subtitle", t("reasons", f.Reason),
			"description", t("statuses", f.Status)+" "+t("meanings", f.Kind)+" "+t("motives", f.Reason),
		),
	)
}
