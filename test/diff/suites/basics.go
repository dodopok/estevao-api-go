package suites

import "github.com/dodopok/estevao-api-go/test/diff"

// basics: the health checks, the API index and the Rswag mounts at
// /api-docs (UI, static assets and OpenAPI files), including the paths where
// both engines cascade to the router's 404.
func init() {
	register("basics", func() []diff.Request {
		get := func(path string, h ...string) diff.Request {
			headers := map[string]string{}
			for i := 0; i+1 < len(h); i += 2 {
				headers[h[i]] = h[i+1]
			}
			return diff.Request{Path: path, Headers: headers}
		}
		method := func(m, path string) diff.Request {
			return diff.Request{Method: m, Path: path, Headers: map[string]string{}}
		}
		indexETag := `W/"22d3fa618766ebceb229c5e96b6ce3c1"`
		return []diff.Request{
			get("/up"), get("/ready"), get("/"), get("/?format=json"), method("HEAD", "/up"),
			get("/api-docs"), get("/api-docs/"), method("HEAD", "/api-docs"), method("POST", "/api-docs"),
			get("/api-docs/index.html"), method("HEAD", "/api-docs/index.html"),
			get("/api-docs/index.html", "If-None-Match", indexETag),
			get("/api-docs/v1/swagger.yaml"), get("/api-docs/v2/swagger.yaml"), method("HEAD", "/api-docs/v1/swagger.yaml"),
			method("POST", "/api-docs/v1/swagger.yaml"), get("/api-docs/v1/../v2/swagger.yaml"), get("/api-docs/v1/%73wagger.yaml"),
			get("/api-docs/v3/swagger.yaml"), get("/api-docs/../config/database.yml"),
			get("/api-docs/index.css"), get("/api-docs/swagger-ui.css"), get("/api-docs/favicon-32x32.png"),
			get("/api-docs/swagger-initializer.js"), get("/api-docs/package.json"), get("/api-docs/swagger-ui.js.map"),
			get("/api-docs/./index.css"), get("/api-docs/x/../index.css"), get("/api-docs/%69ndex.css"),
			method("OPTIONS", "/api-docs/index.css"), method("DELETE", "/api-docs/index.css"), method("HEAD", "/api-docs/index.css"),
			get("/api-docs/index.css", "Range", "bytes=0-9"), get("/api-docs/index.css", "Range", "bytes=-5"),
			get("/api-docs/index.css", "Range", "bytes=10-"), get("/api-docs/index.css", "Range", "bytes=500-"),
			get("/api-docs/index.css", "Range", "bytes=9-3"), get("/api-docs/index.css", "Range", "items=0-9"),
			get("/api-docs/index.css", "If-Modified-Since", "Thu, 24 Sep 2026 02:53:57 GMT"),
			get("/api-docs/index.css", "If-Modified-Since", "Thu, 24 Sep 2020 02:53:57 GMT"),
			get("/api-docs/missing.js"), get("/api-docs/v1"), get("/api-docsx"),
			get("/nope.yml"), get("/nope.yaml"), get("/nope.csv"), get("/nope.json"), get("/nope?format=yml"), get("/nope?format=png"),
		}
	})
}
