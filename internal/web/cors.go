package web

import (
	"net/http"
	"strings"
)

// CORS ports the rack-cors configuration in config/initializers/cors.rb:
// one resource "*" with the listed headers/methods/expose, origins from
// CORS_ALLOWED_ORIGINS (none in production when unset).
type CORS struct {
	Origins []string // "*" allows any
}

var corsMethods = "GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD"
var corsAllowHeaders = []string{"accept", "authorization", "content-type", "x-api-key", "x-app-internal-id"}
var corsExpose = "Content-Type, Content-Disposition, X-Request-Id"

func (c *CORS) allowed(origin string) (string, bool) {
	for _, o := range c.Origins {
		if o == "*" {
			return "*", true
		}
		if o == origin {
			return origin, true
		}
	}
	return "", false
}

// Preflight answers OPTIONS requests carrying Origin and
// Access-Control-Request-Method, like Rack::Cors.
func (c *CORS) Preflight(r *http.Request) *Response {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("X-Origin")
	}
	if origin == "" || r.Method != http.MethodOptions || r.Header.Get("Access-Control-Request-Method") == "" {
		return nil
	}
	h := http.Header{}
	if allow, ok := c.allowed(origin); ok {
		reqMethod := strings.ToLower(r.Header.Get("Access-Control-Request-Method"))
		if !strings.Contains(strings.ToLower(corsMethods), reqMethod) {
			return &Response{Status: 200, Header: h, Body: []byte{}}
		}
		reqHeaders := r.Header.Get("Access-Control-Request-Headers")
		for _, rh := range strings.Split(reqHeaders, ",") {
			rh = strings.ToLower(strings.TrimSpace(rh))
			if rh == "" {
				continue
			}
			ok := false
			for _, a := range corsAllowHeaders {
				if a == rh {
					ok = true
				}
			}
			if !ok {
				return &Response{Status: 200, Header: h, Body: []byte{}}
			}
		}
		h.Set("Access-Control-Allow-Origin", allow)
		h.Set("Access-Control-Allow-Methods", corsMethods)
		h.Set("Access-Control-Expose-Headers", corsExpose)
		h.Set("Access-Control-Max-Age", "7200")
		if reqHeaders != "" {
			h.Set("Access-Control-Allow-Headers", reqHeaders)
		}
	}
	return &Response{Status: 200, Header: h, Body: []byte{}}
}

// Decorate adds the CORS response headers and Vary: Origin.
func (c *CORS) Decorate(r *http.Request, resp *Response) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("X-Origin")
	}
	if origin != "" {
		if allow, ok := c.allowed(origin); ok {
			if resp.Header.Get("Access-Control-Allow-Origin") == "" {
				resp.Header.Set("Access-Control-Allow-Origin", allow)
			}
			if resp.Header.Get("Access-Control-Expose-Headers") == "" {
				resp.Header.Set("Access-Control-Expose-Headers", corsExpose)
			}
		}
	}
	vary := resp.Header.Get("Vary")
	parts := []string{}
	if vary != "" {
		for _, p := range strings.Split(vary, ",") {
			parts = append(parts, strings.TrimSpace(p))
		}
	}
	has := false
	for _, p := range parts {
		if p == "Origin" {
			has = true
		}
	}
	if !has {
		parts = append(parts, "Origin")
	}
	resp.Header.Set("Vary", strings.Join(parts, ", "))
}
