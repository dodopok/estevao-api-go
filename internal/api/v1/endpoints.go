package v1

import "github.com/dodopok/estevao-api-go/internal/web"

// Endpoints lists the v1 actions registered as ApplicationController
// descendants (filled in as controllers are ported).
func Endpoints() map[string]web.HandlerFunc {
	return map[string]web.HandlerFunc{
		"api/v1/calendar#today":          CalendarToday,
		"api/v1/calendar#day":            CalendarDay,
		"api/v1/lectionary#day":          LectionaryDay,
		"api/v1/lectionary#all_services": LectionaryAllServices,
		"api/v1/lectionary#cycle_info":   LectionaryCycleInfo,
	}
}
