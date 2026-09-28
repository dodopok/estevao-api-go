package rosary

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/integrations"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// StrapiError is Integrations::Strapi::Client::Error (an
// ExternalServiceUnavailable); Status is context[:status] (0 when absent).
type StrapiError struct {
	Message string
	Code    string
	Status  int
}

func (e *StrapiError) Error() string { return e.Message }

// strapiClient ports Integrations::Strapi::Client.
type strapiClient struct {
	baseURL, internalToken, readToken string
}

func newStrapiClient() *strapiClient {
	presence := func(k string) string {
		v := config.Get(k)
		if strings.TrimSpace(v) == "" {
			return ""
		}
		return v
	}
	internal := presence("ROSARY_INTERNAL_TOKEN")
	if internal == "" {
		internal = presence("INTERNAL_API_TOKEN")
	}
	return &strapiClient{
		baseURL:       strings.TrimSuffix(config.Get("STRAPI_API_URL"), "/"),
		internalToken: internal,
		readToken:     presence("STRAPI_API_TOKEN"),
	}
}

func (c *strapiClient) post(ctx context.Context, path string, body []byte, failure, code string) (any, error) {
	if strings.TrimSpace(c.baseURL) == "" || c.internalToken == "" {
		return nil, &StrapiError{Message: "Strapi is not configured", Code: "STRAPI_NOT_CONFIGURED"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/"+path, bytes.NewReader(body))
	if err != nil {
		return nil, &StrapiError{Message: "Strapi unavailable", Code: "STRAPI_UNAVAILABLE"}
	}
	req.Header.Set("Authorization", "Bearer "+c.internalToken)
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, failure, code)
}

func (c *strapiClient) do(req *http.Request, failure, code string) (any, error) {
	resp, err := integrations.Client("STRAPI_HTTP_TIMEOUT", integrations.DefaultTimeout).Do(req)
	if err != nil {
		return nil, &StrapiError{Message: "Strapi unavailable", Code: "STRAPI_UNAVAILABLE"}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &StrapiError{Message: "Strapi unavailable", Code: "STRAPI_UNAVAILABLE"}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &StrapiError{Message: failure, Code: code, Status: resp.StatusCode}
	}
	if rb.BlankString(string(raw)) {
		return rb.NewMap(), nil
	}
	v, err := rb.ParseJSON(raw)
	if err != nil {
		return nil, &StrapiError{Message: "Strapi returned invalid JSON", Code: "STRAPI_INVALID_RESPONSE"}
	}
	return v, nil
}

// publish ports Client#publish.
func (c *strapiClient) publish(ctx context.Context, payload *rb.Map) (any, error) {
	return c.post(ctx, "api/internal/rosary-prayers/upsert-approved", rb.ToJSON(payload),
		"Strapi publication failed", "STRAPI_PUBLICATION_FAILED")
}

// unpublish ports Client#unpublish.
func (c *strapiClient) unpublish(ctx context.Context, documentID string, sourceID *int64) (any, error) {
	body := rb.M("document_id", documentID)
	if sourceID != nil {
		body.Set("source_api_prayer_id", *sourceID)
	}
	return c.post(ctx, "api/internal/rosary-prayers/unpublish", rb.ToJSON(body),
		"Strapi unpublication failed", "STRAPI_UNPUBLICATION_FAILED")
}
