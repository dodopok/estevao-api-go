package web

import (
	"context"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Context is one request as a controller sees it.
type Context struct {
	R          *http.Request
	Ctx        context.Context
	RequestID  string
	Endpoint   string
	PathParams map[string]string
	Format     string
	Controller string
	Action     string

	params    *rb.Map
	query     *rb.Map
	body      *rb.Map
	rawBody   []byte
	paramsErr error

	Status  int
	Header  http.Header
	Body    []byte
	written bool
	halted  bool

	// KeepRequestIDs makes a Live response still carry X-Request-Id and
	// X-Runtime (a head after the stream was committed).
	KeepRequestIDs bool

	// Values carries per-request state set by filters (current user, key...).
	Values map[string]any
}

// Halt stops the filter chain after a filter rendered (like a before_action
// that calls render).
type haltSignal struct{}

// Halt aborts the action after a render.
func (c *Context) Halt() { panic(haltSignal{}) }

// Rendered reports whether a response body was set.
func (c *Context) Rendered() bool { return c.written }

// JSON renders v as JSON with the given status (render json:).
func (c *Context) JSON(status int, v any) {
	c.Status = status
	c.Body = rb.JSON(v)
	if c.Header.Get("Content-Type") == "" || !c.written {
		c.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	c.written = true
}

// RenderJSON renders then halts the chain (render + return in a filter).
func (c *Context) RenderJSON(status int, v any) {
	c.JSON(status, v)
	c.Halt()
}

// Head renders an empty body (head :no_content etc).
func (c *Context) HeadStatus(status int) {
	c.Status = status
	c.Body = nil
	// ActionController#head leaves an empty body, which Rack reports as
	// Content-Length: 0 even on 204/304 (unlike Rack::ConditionalGet's 304).
	c.Header.Set("Content-Length", "0")
	c.written = true
}

// Raw renders bytes with a content type.
func (c *Context) Raw(status int, contentType string, body []byte) {
	c.Status = status
	c.Body = body
	c.Header.Set("Content-Type", contentType)
	c.written = true
}

// Param returns params[key] as a Ruby-ish value (string, *rb.Map, []any, nil).
func (c *Context) Param(key string) any { return c.Params().Get(key) }

// ParamS returns params[key].to_s.
func (c *Context) ParamS(key string) string { return rb.ToS(c.Params().Get(key)) }

// ParamPresent mirrors params[key].present?.
func (c *Context) ParamPresent(key string) bool { return rb.Present(c.Params().Get(key)) }

// Params returns the merged parameters (body, query, path), parsing lazily
// like ActionDispatch. Parse failures panic the same way Rails raises.
func (c *Context) Params() *rb.Map {
	if c.params != nil {
		return c.params
	}
	q := c.QueryParams()
	b := c.BodyParams()
	merged := b.Merge(q)
	for k, v := range c.PathParams {
		merged.Set(k, v)
	}
	merged.Set("controller", c.Controller)
	merged.Set("action", c.Action)
	c.params = merged
	return merged
}

// QueryParams returns the parsed query string.
func (c *Context) QueryParams() *rb.Map {
	if c.query != nil {
		return c.query
	}
	q, err := ParseNestedQuery(c.R.URL.RawQuery)
	if err != nil {
		panic(exceptionStatus(400))
	}
	c.query = q
	return q
}

// RawBody returns the request body bytes.
func (c *Context) RawBody() []byte {
	if c.rawBody == nil && c.R.Body != nil {
		b, _ := io.ReadAll(io.LimitReader(c.R.Body, 64<<20))
		c.rawBody = b
		if c.rawBody == nil {
			c.rawBody = []byte{}
		}
	}
	return c.rawBody
}

// BodyParams parses JSON or form bodies like ActionDispatch.
func (c *Context) BodyParams() *rb.Map {
	if c.body != nil {
		return c.body
	}
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(c.R.Header.Get("Content-Type"), ";", 2)[0]))
	raw := c.RawBody()
	m := rb.NewMap()
	switch {
	case ct == "application/json" || strings.HasSuffix(ct, "+json"):
		if len(strings.TrimSpace(string(raw))) > 0 {
			v, err := rb.ParseJSON(raw)
			if err != nil {
				panic(exceptionStatus(500))
			}
			if mm, ok := v.(*rb.Map); ok {
				m = mm
			} else {
				m.Set("_json", v)
			}
		}
	case ct == "multipart/form-data":
		_, mp, err := mime.ParseMediaType(c.R.Header.Get("Content-Type"))
		if err != nil || mp["boundary"] == "" {
			panic(exceptionStatus(400))
		}
		p, err := ParseMultipart(raw, mp["boundary"])
		if err != nil {
			panic(exceptionStatus(400))
		}
		m = p
	case ct == "application/x-www-form-urlencoded":
		p, err := ParseNestedQuery(string(raw))
		if err != nil {
			panic(exceptionStatus(400))
		}
		m = p
	}
	c.body = m
	return m
}

// SetParam overrides a merged parameter (used by params wrapping).
func (c *Context) SetParam(key string, v any) { c.Params().Set(key, v) }

// Get/Set per-request values.
func (c *Context) Get(key string) any { return c.Values[key] }
func (c *Context) Set(key string, v any) {
	if c.Values == nil {
		c.Values = map[string]any{}
	}
	c.Values[key] = v
}

// HeaderValue returns a request header ("" when absent).
func (c *Context) HeaderValue(name string) string { return c.R.Header.Get(name) }

// HasHeader reports whether the request carries the header.
func (c *Context) HasHeader(name string) bool {
	_, ok := c.R.Header[http.CanonicalHeaderKey(name)]
	return ok
}

// Redirect ports redirect_to(url, status:): Location and an empty HTML body.
func (c *Context) Redirect(status int, location string) {
	c.Status = status
	c.Body = nil
	c.Header.Set("Location", location)
	c.Header.Set("Content-Type", "text/html; charset=utf-8")
	c.written = true
}

// HeadBase ports head from a before_action of an ActionController::Base
// controller: an empty text/html response.
func (c *Context) HeadBase(status int) {
	c.HeadStatus(status)
	c.Header.Set("Content-Type", "text/html")
}

// HeadFormat ports head inside an action of an ActionController::Base
// controller: the content type is the route format's (the path extension),
// text/html without one.
func (c *Context) HeadFormat(status int) {
	c.HeadStatus(status)
	ct := "text/html"
	if c.Format != "" {
		ct = ""
		for mime, sym := range mimeSymbols {
			if sym == c.Format && (ct == "" || mime < ct) {
				ct = mime
			}
		}
		if m, ok := extensionMimes[c.Format]; ok {
			ct = m
		}
		if ct == "" {
			ct = "text/html"
		}
	}
	c.Header.Set("Content-Type", ct)
}

// extensionMimes are the registered types whose symbol is not unique.
var extensionMimes = map[string]string{
	"html": "text/html", "text": "text/plain", "txt": "text/plain", "js": "text/javascript", "xml": "application/xml",
	"json": "application/json", "yaml": "application/x-yaml", "jpeg": "image/jpeg", "jpg": "image/jpeg",
	"png": "image/png", "gif": "image/gif", "pdf": "application/pdf", "csv": "text/csv",
}

// SetDate sets the Date header, as expires_in does.
func (c *Context) SetDate() { c.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat)) }
