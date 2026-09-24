package web

import (
	"regexp"
	"strconv"
	"strings"
)

// The app runs behind config.assume_ssl, which sets HTTPS=on,
// X-Forwarded-Port=443 and X-Forwarded-Proto=https on every request, so the
// scheme is always https and the standard port 443.

var (
	trailingPort     = regexp.MustCompile(`:(\d+)\z`)
	forwardedHostSep = regexp.MustCompile(`,\s?`)
)

// rubySplit is String#split(regexp): trailing empty fields are dropped.
func rubySplit(re *regexp.Regexp, s string) []string {
	parts := re.Split(s, -1)
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// URLOptions ports ActionDispatch::Http::URL#host and #optional_port (port
// 0 when it is the standard one): the last X-Forwarded-Host when present,
// else the Host header. The Forwarded header is not consulted: ActionDispatch
// overrides Rack's host_with_port.
func (c *Context) URLOptions() (host string, port int) {
	raw := c.R.Host
	if xs, ok := c.R.Header["X-Forwarded-Host"]; ok {
		if v := strings.Join(xs, ","); strings.TrimSpace(v) != "" {
			if parts := rubySplit(forwardedHostSep, v); len(parts) > 0 {
				raw = parts[len(parts)-1]
			}
		}
	}
	host = trailingPort.ReplaceAllString(raw, "")
	port = 443
	if m := trailingPort.FindStringSubmatch(raw); m != nil {
		port, _ = strconv.Atoi(m[1])
	}
	if port == 443 {
		port = 0
	}
	return host, port
}

// BaseURL ports request.base_url: Rack's "#{scheme}://#{host_with_port}"
// with ActionDispatch's host_with_port.
func (c *Context) BaseURL() string {
	host, port := c.URLOptions()
	if port != 0 {
		return "https://" + host + ":" + strconv.Itoa(port)
	}
	return "https://" + host
}
