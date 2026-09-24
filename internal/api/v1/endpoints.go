package v1

import "github.com/dodopok/estevao-api-go/internal/web"

// Endpoints lists the v1 actions registered as ApplicationController
// descendants (filled in as controllers are ported).
func Endpoints() map[string]web.HandlerFunc {
	return map[string]web.HandlerFunc{
		"api/v1/calendar#today":               CalendarToday,
		"api/v1/calendar#day":                 CalendarDay,
		"api/v1/lectionary#day":               LectionaryDay,
		"api/v1/lectionary#all_services":      LectionaryAllServices,
		"api/v1/prayer_books#index":           PrayerBooksIndex,
		"api/v1/prayer_books#show":            PrayerBooksShow,
		"api/v1/bible_versions#index":         BibleVersionsIndex,
		"api/v1/celebrations#index":           CelebrationsIndex,
		"api/v1/celebrations#show":            CelebrationsShow,
		"api/v1/celebrations#search":          CelebrationsSearch,
		"api/v1/celebrations#by_date":         CelebrationsByDate,
		"api/v1/celebrations#types":           CelebrationsTypes,
		"api/v1/liturgical_explanations#show": LiturgicalExplanationsShow,
		"api/v1/lectionary#cycle_info":        LectionaryCycleInfo,
		"api/v1/daily_office#today":           DailyOfficeToday,
		"api/v1/daily_office#show":            DailyOfficeShow,
		"api/v1/daily_office#family_rite":     DailyOfficeFamilyRite,
		"api/v1/daily_office#preferences":     DailyOfficePreferences,
		"api/v1/users#show":                   UsersShow,
		"api/v1/users#update_profile":         UsersUpdateProfile,
		"api/v1/users#upload_avatar":          UsersUploadAvatar,
		"api/v1/users#delete_avatar":          UsersDeleteAvatar,
		"api/v1/users#update_preferences":     UsersUpdatePreferences,
		"api/v1/users#completions":            UsersCompletions,
		"api/v1/users#save_fcm_token":         UsersSaveFCMToken,
		"api/v1/users#delete_fcm_token":       UsersDeleteFCMToken,
		"api/v1/users#update_timezone":        UsersUpdateTimezone,
		"api/v1/users#destroy":                UsersDestroy,
		"api/v1/users/wrapped#show":           UsersWrappedShow,
		"api/v1/onboarding#create":            OnboardingCreate,
		"api/v1/onboarding#show":              OnboardingShow,
	}
}
