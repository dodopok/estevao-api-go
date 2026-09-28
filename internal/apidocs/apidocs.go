// Package apidocs serves what the Rails app mounts at /api-docs: the Swagger
// UI (Rswag::Ui, a Rack::Static over swagger-ui-dist plus a rendered index)
// and the OpenAPI files (Rswag::Api). The assets are generated from the Rails
// app by tools/gen/api_docs.rb.
package apidocs

import (
	"embed"
	"encoding/json"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/web"
)

//go:embed assets
var assets embed.FS

type apiFile struct {
	Path   string            `json:"path"`
	Type   string            `json:"type"`
	Bodies map[string]string `json:"bodies"`
}

type uiFile struct {
	Path  string `json:"path"`
	File  string `json:"file"`
	Type  string `json:"type"`
	MTime string `json:"mtime"`
}

var (
	apiFiles = map[string]apiFile{}
	uiFiles  = map[string]uiFile{}
	index    []byte
)

func init() {
	raw, err := assets.ReadFile("assets/manifest.json")
	if err != nil {
		panic(err)
	}
	var m struct {
		API []apiFile `json:"api"`
		UI  []uiFile  `json:"ui"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(err)
	}
	for _, f := range m.API {
		apiFiles[f.Path] = f
	}
	for _, f := range m.UI {
		uiFiles[f.Path] = f
	}
	if index, err = assets.ReadFile("assets/ui_index.html"); err != nil {
		panic(err)
	}
}

const csp = "default-src 'self'; img-src 'self' data: https://validator.swagger.io; font-src 'self' https://fonts.gstatic.com; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; script-src 'self' 'unsafe-inline'; "

// Serve answers a request under /api-docs as the two mounted engines do, or
// returns nil when both cascade (the router then answers 404).
func Serve(r *http.Request) *web.Response {
	raw := r.URL.EscapedPath()
	var sub string
	switch {
	case raw == "/api-docs":
		sub = "/"
	case strings.HasPrefix(raw, "/api-docs/"):
		sub = strings.TrimPrefix(raw, "/api-docs")
	default:
		return nil
	}
	if resp := ui(r, sub); resp != nil {
		return resp
	}
	return api(r, sub)
}

// ui ports Rswag::Ui::Middleware (a Rack::Static over the assets root).
func ui(r *http.Request, sub string) *web.Response {
	if r.Method == http.MethodGet && sub == "/" {
		h := http.Header{}
		h.Set("Location", "/api-docs/index.html")
		return &web.Response{Status: 301, Header: h, Body: []byte{}}
	}
	if r.Method == http.MethodGet && sub == "/index.html" {
		h := http.Header{}
		h.Set("Content-Type", "text/html")
		h.Set("Content-Security-Policy", csp)
		return &web.Response{Status: 200, Header: h, Body: index, KeepTypeOn304: true}
	}
	return files(r, sub)
}

// files ports Rack::Files: GET, HEAD and OPTIONS of a file under the root,
// a single byte range; anything it cannot serve cascades.
func files(r *http.Request, sub string) *web.Response {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
	default:
		return nil
	}
	p, err := unescapePath(sub)
	if err != nil || strings.Contains(p, "\x00") {
		return nil
	}
	f, ok := uiFiles[cleanPathInfo(p)]
	if !ok {
		return nil
	}
	if r.Method == http.MethodOptions {
		h := http.Header{}
		h.Set("Allow", "GET, HEAD, OPTIONS")
		return &web.Response{Status: 200, Header: h, Body: []byte{}}
	}
	body, err := assets.ReadFile("assets/" + f.File)
	if err != nil {
		return nil
	}
	h := http.Header{}
	h.Set("Last-Modified", f.MTime)
	h.Set("Content-Type", f.Type)
	if rng := r.Header.Get("Range"); rng != "" {
		start, end, ok := byteRange(rng, len(body))
		if ok && start < 0 {
			return nil // unsatisfiable: Rack::Files fails with a cascade
		}
		if ok {
			h.Set("Content-Range", "bytes "+strconv.Itoa(start)+"-"+strconv.Itoa(end)+"/"+strconv.Itoa(len(body)))
			return &web.Response{Status: 206, Header: h, Body: body[start : end+1]}
		}
	}
	return &web.Response{Status: 200, Header: h, Body: body, HeadLength: true}
}

// api ports Rswag::Api::Middleware: GET of an OpenAPI file, filtered for the
// environment (the Rack status is the String "200", so Rack::ETag skips it).
func api(r *http.Request, sub string) *web.Response {
	if r.Method != http.MethodGet {
		return nil
	}
	f, ok := apiFiles[path.Clean("/"+sub)]
	if !ok {
		return nil
	}
	env := "development"
	if config.RailsEnv() == "production" {
		env = "production"
	}
	body, err := assets.ReadFile("assets/" + f.Bodies[env])
	if err != nil {
		return nil
	}
	h := http.Header{}
	h.Set("Content-Type", f.Type)
	return &web.Response{Status: 200, Header: h, Body: body, NoETag: true}
}

// cleanPathInfo ports Rack::Utils.clean_path_info.
func cleanPathInfo(p string) string {
	var clean []string
	for _, part := range strings.Split(p, "/") {
		switch part {
		case "", ".":
		case "..":
			if len(clean) > 0 {
				clean = clean[:len(clean)-1]
			}
		default:
			clean = append(clean, part)
		}
	}
	return "/" + strings.Join(clean, "/")
}

// unescapePath ports Rack::Utils.unescape_path (%XX only).
func unescapePath(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			n, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
			if err == nil {
				b.WriteByte(byte(n))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String(), nil
}

// byteRange ports Rack::Utils.get_byte_ranges for one range: ok=false when
// the header is not a byte range (the whole file is served); start<0 when
// no range is satisfiable. Several ranges are served whole.
func byteRange(header string, size int) (start, end int, ok bool) {
	spec, found := strings.CutPrefix(strings.TrimSpace(header), "bytes=")
	if !found {
		return 0, 0, false
	}
	parts := strings.Split(spec, ",")
	if len(parts) != 1 {
		return 0, 0, false
	}
	first, last, found := strings.Cut(strings.TrimSpace(parts[0]), "-")
	if !found {
		return 0, 0, false
	}
	first, last = strings.TrimSpace(first), strings.TrimSpace(last)
	if first == "" {
		if last == "" {
			return 0, 0, false
		}
		n, err := strconv.Atoi(last)
		if err != nil {
			return 0, 0, false
		}
		if n == 0 {
			return -1, 0, true
		}
		start, end = max(size-n, 0), size-1
	} else {
		s, err := strconv.Atoi(first)
		if err != nil {
			return 0, 0, false
		}
		e := size - 1
		if last != "" {
			if e, err = strconv.Atoi(last); err != nil {
				return 0, 0, false
			}
			if e < s {
				return 0, 0, false
			}
			e = min(e, size-1)
		}
		start, end = s, e
	}
	if start > end || start >= size {
		return -1, 0, true
	}
	return start, end, true
}
