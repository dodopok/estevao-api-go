package web

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// HandlerFunc is a controller action.
type HandlerFunc func(c *Context)

// Endpoint describes how an action is dispatched.
type Endpoint struct {
	Handler HandlerFunc
	// Application marks ApplicationController descendants: they get the
	// JSON headers filter and the production rescue_from StandardError.
	Application bool
	// Plain skips the controller default headers (ActionController::API
	// subclasses not inheriting ApplicationController still get them; only
	// Rack-level endpoints set this).
	Plain bool
}

// Response is a finished HTTP response inside the middleware chain.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
	// Static responses are served before Runtime/RequestId (no ids added).
	Static bool
}

// Middleware may answer before the application (Rack::Attack).
type Middleware func(r *http.Request) *Response

// Server is the Rails-compatible HTTP stack.
type Server struct {
	Router    *Router
	Endpoints map[string]Endpoint
	Attack    Middleware
	Static    func(r *http.Request) *Response
	CORS      *CORS
	Logger    *slog.Logger
	// ReportError receives unexpected failures (the New Relic hook).
	ReportError func(c *Context, err any, stack []byte)
}

var defaultHeaders = [][2]string{
	{"X-Frame-Options", "SAMEORIGIN"},
	{"X-Xss-Protection", "0"},
	{"X-Content-Type-Options", "nosniff"},
	{"X-Permitted-Cross-Domain-Policies", "none"},
	{"Referrer-Policy", "strict-origin-when-cross-origin"},
}

var requestIDSanitizer = regexp.MustCompile(`[^\w\-@]`)

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// ServeHTTP runs the stack: Cors > SSL > Runtime > RequestId >
// ShowExceptions > Head > ConditionalGet > ETag > Attack > routes.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if s.CORS != nil {
		if resp := s.CORS.Preflight(r); resp != nil {
			writeResponse(w, r, resp)
			return
		}
	}
	if s.Static != nil {
		if resp := s.Static(r); resp != nil {
			resp.Header.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			if s.CORS != nil {
				s.CORS.Decorate(r, resp)
			}
			writeResponse(w, r, resp)
			return
		}
	}
	reqID := newUUID()
	if raw := r.Header.Get("X-Request-Id"); strings.TrimSpace(raw) != "" {
		reqID = requestIDSanitizer.ReplaceAllString(raw, "")
		if len(reqID) > 255 {
			reqID = reqID[:255]
		}
	}
	resp := s.showExceptions(r, reqID)
	resp.Header.Set("X-Request-Id", reqID)
	resp.Header.Set("X-Runtime", fmt.Sprintf("%0.6f", time.Since(start).Seconds()))
	resp.Header.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
	if s.CORS != nil {
		s.CORS.Decorate(r, resp)
	}
	writeResponse(w, r, resp)
}

func writeResponse(w http.ResponseWriter, r *http.Request, resp *Response) {
	h := w.Header()
	for k, v := range resp.Header {
		h[k] = v
	}
	if r.Method == http.MethodHead {
		h.Set("Content-Length", "0")
		w.WriteHeader(resp.Status)
		return
	}
	if resp.Status == 204 || resp.Status == 304 || (resp.Status >= 100 && resp.Status < 200) {
		h.Del("Content-Length")
		w.WriteHeader(resp.Status)
		return
	}
	h.Set("Content-Length", strconv.Itoa(len(resp.Body)))
	w.WriteHeader(resp.Status)
	_, _ = w.Write(resp.Body)
}

// showExceptions renders errors escaping the inner stack through the
// public exceptions app.
func (s *Server) showExceptions(r *http.Request, reqID string) (resp *Response) {
	defer func() {
		if rec := recover(); rec != nil {
			status := 500
			if es, ok := rec.(exceptionStatus); ok {
				status = int(es)
			} else if s.Logger != nil {
				s.Logger.Error("unhandled error", "error", fmt.Sprint(rec), "request_id", reqID, "stack", string(debug.Stack()))
			}
			resp = PublicException(r, status)
		}
	}()
	resp = s.conditional(r, reqID)
	return resp
}

// conditional applies Rack::ETag then Rack::ConditionalGet.
func (s *Server) conditional(r *http.Request, reqID string) *Response {
	resp := s.inner(r, reqID)
	var digest string
	if (resp.Status == 200 || resp.Status == 201) && resp.Header.Get("Etag") == "" && resp.Header.Get("Last-Modified") == "" && len(resp.Body) > 0 {
		sum := sha256.Sum256(resp.Body)
		digest = hex.EncodeToString(sum[:])[:32]
		resp.Header.Set("Etag", `W/"`+digest+`"`)
	}
	if resp.Header.Get("Cache-Control") == "" {
		if digest != "" {
			resp.Header.Set("Cache-Control", "max-age=0, private, must-revalidate")
		} else {
			resp.Header.Set("Cache-Control", "no-cache")
		}
	}
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && resp.Status == 200 {
		if fresh(r, resp.Header) {
			resp.Status = 304
			resp.Header.Del("Content-Type")
			resp.Header.Del("Content-Length")
			resp.Body = nil
		}
	}
	return resp
}

func fresh(r *http.Request, h http.Header) bool {
	if nm, ok := r.Header["If-None-Match"]; ok {
		return h.Get("Etag") == strings.Join(nm, ", ")
	}
	if ims := r.Header.Get("If-Modified-Since"); ims != "" {
		since, err := http.ParseTime(ims)
		if err != nil {
			return false
		}
		lm := h.Get("Last-Modified")
		if lm == "" {
			return false
		}
		last, err := http.ParseTime(lm)
		if err != nil {
			return false
		}
		return !since.Before(last)
	}
	return false
}

func (s *Server) inner(r *http.Request, reqID string) *Response {
	if s.Attack != nil {
		if resp := s.Attack(r); resp != nil {
			return resp
		}
	}
	m, ok := s.Router.Recognize(r.Method, r.URL.EscapedPath())
	if !ok {
		panic(exceptionStatus(404))
	}
	ep, ok := s.Endpoints[m.Endpoint]
	if !ok {
		// A route exists in Rails whose controller is not part of the
		// ported surface (framework engines). Answer as Rails does for an
		// unreachable controller.
		panic(exceptionStatus(404))
	}
	return s.dispatch(r, reqID, m, ep)
}

func (s *Server) dispatch(r *http.Request, reqID string, m *Match, ep Endpoint) *Response {
	controller, action, _ := strings.Cut(m.Endpoint, "#")
	c := &Context{
		R:          r,
		Ctx:        r.Context(),
		RequestID:  reqID,
		Endpoint:   m.Endpoint,
		PathParams: m.Params,
		Format:     m.Format,
		Controller: controller,
		Action:     action,
		Status:     200,
		Header:     http.Header{},
		Values:     map[string]any{},
	}
	if !ep.Plain {
		for _, kv := range defaultHeaders {
			c.Header.Set(kv[0], kv[1])
		}
	}
	if ep.Application {
		c.Header.Set("Content-Type", "application/json; charset=utf-8")
		c.Header.Set("Content-Disposition", "inline")
		c.Header.Set("X-Request-Id", reqID)
	}
	s.run(c, ep)
	if !c.written {
		c.Status = 204
		c.Body = nil
	}
	if c.written && c.Body != nil && c.varyAccept() && c.Header.Get("Vary") == "" {
		c.Header.Set("Vary", "Accept")
	}
	return &Response{Status: c.Status, Header: c.Header, Body: c.Body}
}

func (s *Server) run(c *Context, ep Endpoint) {
	defer func() {
		rec := recover()
		if rec == nil {
			return
		}
		switch e := rec.(type) {
		case haltSignal:
			return
		case exceptionStatus:
			panic(e)
		case *DomainError:
			if !ep.Application {
				panic(e)
			}
			c.JSON(e.Status(), rb.M("error", e.Message, "code", e.Code, "request_id", c.RequestID))
			return
		case *InfraError:
			if !ep.Application {
				panic(e)
			}
			if s.ReportError != nil {
				s.ReportError(c, e, nil)
			}
			c.JSON(e.Status(), rb.M("error", e.Message, "code", e.Code, "request_id", c.RequestID))
			return
		}
		if !ep.Application {
			panic(rec)
		}
		stack := debug.Stack()
		if s.ReportError != nil {
			s.ReportError(c, rec, stack)
		}
		if s.Logger != nil {
			s.Logger.Error("[ERROR]", "error", fmt.Sprint(rec), "endpoint", c.Endpoint, "request_id", c.RequestID, "stack", string(stack))
		}
		msg := fmt.Sprint(rec)
		if se, ok := rec.(*StandardError); ok {
			msg = se.Message
		} else if err, ok := rec.(error); ok {
			msg = err.Error()
		}
		c.JSON(500, rb.M("error", "Internal server error", "message", msg, "trace_id", nil))
	}()
	ep.Handler(c)
}

// varyAccept ports ActionDispatch::Request#should_apply_vary_header?.
func (c *Context) varyAccept() bool {
	if c.Format != "" || rb.Present(c.QueryParams().Get("format")) {
		return false
	}
	accept := c.R.Header.Get("Accept")
	xhr := strings.EqualFold(c.R.Header.Get("X-Requested-With"), "XMLHttpRequest")
	if xhr && (strings.TrimSpace(accept) != "" || c.R.Header.Get("Content-Type") != "") {
		return true
	}
	return strings.TrimSpace(accept) != "" && !browserLikeAccepts.MatchString(accept)
}

var browserLikeAccepts = regexp.MustCompile(`,\s*\*/\*|\*/\*\s*,`)
