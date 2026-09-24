// Package s3 is the S3-compatible object store client behind the Active
// Storage `railway_avatars` service (config/storage.yml): SigV4-signed
// requests and presigned GETs, addressed the way aws-sdk-s3 addresses a
// custom endpoint.
package s3

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Client is one bucket on one endpoint.
type Client struct {
	AccessKey, SecretKey, Region, Bucket, Endpoint string
	ForcePathStyle                                 bool
	HTTP                                           *http.Client
	Now                                            func() time.Time
}

// FromEnv builds the railway_avatars service client (config/storage.yml).
func FromEnv() *Client {
	region := os.Getenv("AVATAR_BUCKET_REGION")
	if _, ok := os.LookupEnv("AVATAR_BUCKET_REGION"); !ok {
		region = "auto"
	}
	return &Client{
		AccessKey: os.Getenv("AVATAR_BUCKET_ACCESS_KEY_ID"), SecretKey: os.Getenv("AVATAR_BUCKET_SECRET_ACCESS_KEY"),
		Region: region, Bucket: os.Getenv("AVATAR_BUCKET_NAME"), Endpoint: os.Getenv("AVATAR_BUCKET_ENDPOINT"),
		HTTP: &http.Client{Timeout: 60 * time.Second}, Now: time.Now,
	}
}

var virtualHostableBucket = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

// target returns the scheme, host and escaped path of key.
func (c *Client) target(key string) (scheme, host, path string) {
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = "https://s3." + c.Region + ".amazonaws.com"
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		panic(err)
	}
	host = u.Host
	base := strings.TrimSuffix(u.Path, "/")
	escapedKey := escapePath(key)
	if c.ForcePathStyle || net.ParseIP(u.Hostname()) != nil || !virtualHostableBucket.MatchString(c.Bucket) {
		path = base + "/" + Escape(c.Bucket)
		if key != "" {
			path += "/" + escapedKey
		}
	} else {
		host = c.Bucket + "." + host
		path = base + "/" + escapedKey
	}
	return u.Scheme, host, path
}

// PresignedGet reproduces aws-sdk-s3's presigned_url(:get) with the
// response-content-disposition and response-content-type overrides Active
// Storage passes (either may be empty to omit it).
func (c *Client) PresignedGet(key string, expiresIn int, disposition, contentType string, now time.Time) string {
	scheme, host, path := c.target(key)
	amzDate := now.UTC().Format("20060102T150405Z")
	scope := amzDate[:8] + "/" + c.Region + "/s3/aws4_request"
	var params [][2]string
	if disposition != "" {
		params = append(params, [2]string{"response-content-disposition", disposition})
	}
	if contentType != "" {
		params = append(params, [2]string{"response-content-type", contentType})
	}
	params = append(params,
		[2]string{"X-Amz-Algorithm", "AWS4-HMAC-SHA256"},
		[2]string{"X-Amz-Credential", c.AccessKey + "/" + scope},
		[2]string{"X-Amz-Date", amzDate},
		[2]string{"X-Amz-Expires", fmt.Sprint(expiresIn)},
		[2]string{"X-Amz-SignedHeaders", "host"},
	)
	sorted := append([][2]string(nil), params...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i][0] < sorted[j][0] })
	canonical := strings.Join([]string{"GET", path, encodeParams(sorted), "host:" + host + "\n", "host", "UNSIGNED-PAYLOAD"}, "\n")
	signature := c.signature(amzDate, scope, canonical)
	return scheme + "://" + host + path + "?" + encodeParams(params) + "&X-Amz-Signature=" + signature
}

func (c *Client) signature(amzDate, scope, canonical string) string {
	digest := sha256.Sum256([]byte(canonical))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(digest[:])
	k := mac([]byte("AWS4"+c.SecretKey), amzDate[:8])
	k = mac(k, c.Region)
	k = mac(k, "s3")
	k = mac(k, "aws4_request")
	return hex.EncodeToString(mac(k, stringToSign))
}

func mac(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func encodeParams(params [][2]string) string {
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = Escape(p[0]) + "=" + Escape(p[1])
	}
	return strings.Join(parts, "&")
}

// Escape is Seahorse::Util.uri_escape: everything but A-Z a-z 0-9 - _ . ~
// percent-encoded.
func Escape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = Escape(part)
	}
	return strings.Join(parts, "/")
}

// SignRequest adds the SigV4 Authorization header (and x-amz-date,
// x-amz-content-sha256) to req, signing every header already set plus host.
func (c *Client) SignRequest(req *http.Request, payload []byte, now time.Time) {
	amzDate := now.UTC().Format("20060102T150405Z")
	scope := amzDate[:8] + "/" + c.Region + "/s3/aws4_request"
	sum := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(sum[:])
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	headers := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		headers[strings.ToLower(k)] = strings.Join(strings.Fields(strings.Join(v, ",")), " ")
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonicalHeaders strings.Builder
	for _, k := range names {
		canonicalHeaders.WriteString(k + ":" + headers[k] + "\n")
	}
	signed := strings.Join(names, ";")
	var query [][2]string
	for k, vs := range req.URL.Query() {
		for _, v := range vs {
			query = append(query, [2]string{k, v})
		}
	}
	sort.Slice(query, func(i, j int) bool {
		if query[i][0] != query[j][0] {
			return query[i][0] < query[j][0]
		}
		return query[i][1] < query[j][1]
	})
	canonical := strings.Join([]string{req.Method, req.URL.EscapedPath(), encodeParams(query), canonicalHeaders.String(), signed, payloadHash}, "\n")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+c.AccessKey+"/"+scope+
		", SignedHeaders="+signed+", Signature="+c.signature(amzDate, scope, canonical))
}

// ErrNotFound is a missing object (ActiveStorage::FileNotFoundError).
var ErrNotFound = errors.New("s3: object not found")

func (c *Client) do(ctx context.Context, method, key, rawQuery string, body []byte, headers map[string]string) (*http.Response, error) {
	scheme, host, path := c.target(key)
	u := &url.URL{Scheme: scheme, Host: host, RawPath: path, RawQuery: rawQuery}
	u.Path, _ = url.PathUnescape(path)
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.ContentLength = int64(len(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	c.SignRequest(req, body, now())
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	return hc.Do(req)
}

func statusError(op string, resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("s3 %s: %s: %s", op, resp.Status, strings.TrimSpace(string(b)))
}

// Put stores an object (Aws::S3::Object#put with content_md5, content_type
// and, when set, content_disposition).
func (c *Client) Put(ctx context.Context, key string, body []byte, md5Base64, contentType, disposition string) error {
	h := map[string]string{"Content-Md5": md5Base64}
	if contentType != "" {
		h["Content-Type"] = contentType
	}
	if disposition != "" {
		h["Content-Disposition"] = disposition
	}
	resp, err := c.do(ctx, http.MethodPut, key, "", body, h)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return statusError("put", resp)
	}
	return nil
}

// Get downloads an object.
func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	resp, err := c.do(ctx, http.MethodGet, key, "", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode/100 != 2 {
		return nil, statusError("get", resp)
	}
	return io.ReadAll(resp.Body)
}

// Delete removes an object (idempotent).
func (c *Client) Delete(ctx context.Context, key string) error {
	resp, err := c.do(ctx, http.MethodDelete, key, "", nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return statusError("delete", resp)
	}
	return nil
}

// DeletePrefixed ports S3Service#delete_prefixed: every object under prefix.
func (c *Client) DeletePrefixed(ctx context.Context, prefix string) error {
	resp, err := c.do(ctx, http.MethodGet, "", "list-type=2&prefix="+Escape(prefix), nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return statusError("list", resp)
	}
	var listing struct {
		Contents []struct{ Key string } `xml:"Contents"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return err
	}
	if len(listing.Contents) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString(`<Delete xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
	for _, o := range listing.Contents {
		b.WriteString("<Object><Key>")
		xml.EscapeText(&b, []byte(o.Key))
		b.WriteString("</Key></Object>")
	}
	b.WriteString("</Delete>")
	body := []byte(b.String())
	del, err := c.do(ctx, http.MethodPost, "", "delete=", body, map[string]string{"Content-Type": "application/xml", "Content-Md5": md5Base64(body)})
	if err != nil {
		return err
	}
	defer del.Body.Close()
	if del.StatusCode/100 != 2 {
		return statusError("delete objects", del)
	}
	return nil
}

func md5Base64(b []byte) string {
	sum := md5.Sum(b)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// PresignedPut ports presigned_url(:put, content_type:, content_length:,
// content_md5:, whitelist_headers: ["content-length"]) as Active Storage
// issues it for a direct upload: those three headers are signed.
func (c *Client) PresignedPut(key string, expiresIn int, contentType string, contentLength int64, md5Base64 string, now time.Time) string {
	scheme, host, path := c.target(key)
	amzDate := now.UTC().Format("20060102T150405Z")
	scope := amzDate[:8] + "/" + c.Region + "/s3/aws4_request"
	signedHeaders := "content-length;content-md5;content-type;host"
	params := [][2]string{
		{"X-Amz-Algorithm", "AWS4-HMAC-SHA256"},
		{"X-Amz-Credential", c.AccessKey + "/" + scope},
		{"X-Amz-Date", amzDate},
		{"X-Amz-Expires", fmt.Sprint(expiresIn)},
		{"X-Amz-SignedHeaders", signedHeaders},
	}
	headers := "content-length:" + fmt.Sprint(contentLength) + "\ncontent-md5:" + md5Base64 + "\ncontent-type:" + contentType + "\nhost:" + host + "\n"
	canonical := strings.Join([]string{"PUT", path, encodeParams(params), headers, signedHeaders, "UNSIGNED-PAYLOAD"}, "\n")
	return scheme + "://" + host + path + "?" + encodeParams(params) + "&X-Amz-Signature=" + c.signature(amzDate, scope, canonical)
}
