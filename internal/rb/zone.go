package rb

import "time"

// RailsZone ports ActiveSupport::TimeZone[name]: a Rails zone name or a tz
// identifier, nil when TZInfo does not know it.
func RailsZone(name string) *time.Location {
	id := name
	if mapped, ok := railsZoneMapping[name]; ok {
		id = mapped
	}
	if id == "" || id == "Local" {
		return nil
	}
	loc, err := time.LoadLocation(id)
	if err != nil {
		return nil
	}
	return loc
}
