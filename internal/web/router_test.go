package web

import "testing"

func TestRecognizeMirrorsRailsOrder(t *testing.T) {
	r := NewRouter(RailsRoutes)
	cases := []struct {
		method, path, endpoint string
		params                 map[string]string
	}{
		{"GET", "/api/v1/calendar/2025/overview", "api/v1/calendar#overview", map[string]string{"year": "2025"}},
		{"GET", "/api/v1/calendar/abcd/overview", "api/v1/calendar#month", map[string]string{"year": "abcd", "month": "overview"}},
		{"GET", "/api/v1/calendar/2025/12/25.json", "api/v1/calendar#day", map[string]string{"year": "2025", "month": "12", "day": "25", "format": "json"}},
		{"GET", "/api/v1/calendar/2025/", "api/v1/calendar#year", map[string]string{"year": "2025"}},
		{"HEAD", "/api/v1/prayer_books", "api/v1/prayer_books#index", map[string]string{}},
		{"DELETE", "/api/v1/favorites/a.b", "api/v1/favorites#destroy", map[string]string{"post_slug": "a.b"}},
		{"GET", "/api/v1/shared_offices/ab.cd", "api/v1/shared_offices#show", map[string]string{"code": "ab", "format": "cd"}},
		{"GET", "/", "home#index", map[string]string{}},
	}
	for _, c := range cases {
		m, ok := r.Recognize(c.method, c.path)
		if !ok {
			t.Fatalf("%s %s not recognized", c.method, c.path)
		}
		if m.Endpoint != c.endpoint {
			t.Errorf("%s: endpoint %s want %s", c.path, m.Endpoint, c.endpoint)
		}
		for k, v := range c.params {
			if m.Params[k] != v {
				t.Errorf("%s: param %s=%q want %q", c.path, k, m.Params[k], v)
			}
		}
	}
	if _, ok := r.Recognize("POST", "/api/v1/calendar/2025"); ok {
		t.Error("POST should not match a GET route")
	}
}
