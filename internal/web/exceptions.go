package web

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// statusTexts are Rack::Utils::HTTP_STATUS_CODES (Rack 3.2).
var statusTexts = map[int]string{
	400: "Bad Request", 401: "Unauthorized", 403: "Forbidden", 404: "Not Found",
	405: "Method Not Allowed", 406: "Not Acceptable", 409: "Conflict", 413: "Content Too Large",
	415: "Unsupported Media Type", 416: "Range Not Satisfiable", 422: "Unprocessable Content", 429: "Too Many Requests",
	500: "Internal Server Error", 501: "Not Implemented", 502: "Bad Gateway", 503: "Service Unavailable",
}

// StatusText returns Rack's reason phrase.
func StatusText(code int) string {
	if s, ok := statusTexts[code]; ok {
		return s
	}
	if s := http.StatusText(code); s != "" {
		return s
	}
	return statusTexts[500]
}

// mimeSymbols maps registered MIME strings to Rails symbols.
var mimeSymbols = map[string]string{
	"text/html": "html", "application/xhtml+xml": "html", "text/plain": "text",
	"text/javascript": "js", "application/javascript": "js", "application/x-javascript": "js",
	"text/css": "css", "text/calendar": "ics", "text/csv": "csv", "text/vcard": "vcf", "text/vtt": "vtt",
	"image/png": "png", "image/jpeg": "jpeg", "image/pjpeg": "jpeg", "image/gif": "gif", "image/bmp": "bmp",
	"image/tiff": "tiff", "image/svg+xml": "svg", "image/webp": "webp", "audio/mpeg": "mpeg",
	"audio/mp3": "mp3", "audio/ogg": "ogg", "audio/aac": "m4a", "video/webm": "webm", "video/mp4": "mp4",
	"font/otf": "otf", "font/ttf": "ttf", "font/woff": "woff", "font/woff2": "woff2",
	"application/xml": "xml", "text/xml": "xml", "application/x-xml": "xml",
	"application/rss+xml": "rss", "application/atom+xml": "atom",
	"application/x-yaml": "yaml", "text/yaml": "yaml",
	"multipart/form-data": "multipart_form", "application/x-www-form-urlencoded": "url_encoded_form",
	"application/json": "json", "text/x-json": "json", "application/jsonrequest": "json",
	"application/pdf": "pdf", "application/zip": "zip", "application/gzip": "gzip",
	"*/*": "all",
}

// mimeByExtension is Mime::Type.lookup_by_extension for renderable formats.
var mimeByExtension = map[string]string{"json": "application/json", "xml": "application/xml", "yaml": "application/x-yaml", "html": "text/html", "text": "text/plain", "txt": "text/plain", "js": "text/javascript"}

var paramSep = regexp.MustCompile(`;\s*\w+="?\w+"?`)

type acceptItem struct {
	index int
	name  string
	q     float64
}

// parseAccept ports Mime::Type.parse (without trailing-star expansion).
func parseAccept(h string) []string {
	if !strings.Contains(h, ",") {
		if loc := paramSep.FindStringIndex(h); loc != nil {
			h = strings.TrimSpace(h[:loc[0]])
		}
		if strings.TrimSpace(h) == "" {
			return nil
		}
		return []string{strings.TrimSpace(h)}
	}
	var items []acceptItem
	for i, part := range strings.Split(h, ",") {
		pieces := paramSep.Split(part, -1)
		name := strings.TrimSpace(pieces[0])
		if name == "" {
			continue
		}
		q := 1.0
		if m := regexp.MustCompile(`;\s*q="?([\d.]+)"?`).FindStringSubmatch(part); m != nil {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				q = v
			}
		}
		items = append(items, acceptItem{index: i, name: name, q: q})
	}
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].q != items[b].q {
			return items[a].q > items[b].q
		}
		return items[a].index < items[b].index
	})
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.name
	}
	return out
}

// requestFormat returns the first negotiated MIME string ("" when none),
// porting ActionDispatch::Request#formats.first for the exceptions app.
func requestFormat(r *http.Request) string {
	q, err := ParseNestedQuery(r.URL.RawQuery)
	if err == nil {
		if f := strings.TrimSpace(fmt.Sprint(q.Get("format"))); q.Get("format") != nil && f != "" {
			if m, ok := mimeByExtension[f]; ok {
				return m
			}
			return ""
		}
	}
	accept := r.Header.Get("Accept")
	xhr := strings.EqualFold(r.Header.Get("X-Requested-With"), "XMLHttpRequest")
	valid := (xhr && (strings.TrimSpace(accept) != "" || r.Header.Get("Content-Type") != "")) ||
		(strings.TrimSpace(accept) != "" && !browserLikeAccepts.MatchString(accept))
	if valid {
		for _, a := range parseAccept(accept) {
			if _, ok := mimeSymbols[a]; ok {
				return a
			}
		}
		return ""
	}
	if m := regexp.MustCompile(`\.(\w+)$`).FindStringSubmatch(r.URL.Path); m != nil {
		if mt, ok := mimeByExtension[m[1]]; ok {
			return mt
		}
	}
	if xhr {
		return "text/javascript"
	}
	return "text/html"
}

// PublicException ports ActionDispatch::PublicExceptions (no public/*.html
// pages exist, so HTML falls through to an empty pass response).
func PublicException(r *http.Request, status int) *Response {
	h := http.Header{}
	ct := requestFormat(r)
	// A routed format (the path extension) wins, as in request.formats.
	if holder, ok := r.Context().Value(routeFormatKey{}).(*string); ok && *holder != "" {
		ct = extensionMimes[*holder]
		if ct == "" {
			for mime, sym := range mimeSymbols {
				if sym == *holder {
					ct = mime
					break
				}
			}
		}
	}
	text := StatusText(status)
	var body string
	switch mimeSymbols[ct] {
	case "json":
		body = fmt.Sprintf(`{"status":%d,"error":"%s"}`, status, text)
	case "xml":
		body = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<hash>\n  <status type=\"integer\">" + strconv.Itoa(status) +
			"</status>\n  <error>" + text + "</error>\n</hash>\n"
	case "yaml":
		body = fmt.Sprintf("---\n:status: %d\n:error: %s\n", status, text)
	default:
		h.Set("Content-Type", "text/html; charset=UTF-8")
		return &Response{Status: status, Header: h, Body: []byte{}}
	}
	if r.Method == http.MethodHead {
		body = ""
	}
	h.Set("Content-Type", ct+"; charset=UTF-8")
	return &Response{Status: status, Header: h, Body: []byte(body)}
}

// routeFormatKey carries the matched route's format to the exceptions app.
type routeFormatKey struct{}
