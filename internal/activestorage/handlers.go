package activestorage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/s3"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// setBlob ports ActiveStorage::SetBlob: find_signed! or head :not_found.
// An unknown id raises ActiveRecord::RecordNotFound, which production renders
// as the public 404.
func setBlob(c *web.Context, param string) *Blob {
	id, ok := Verified(c.ParamS(param), "blob_id", time.Now())
	if !ok {
		c.HeadBase(http.StatusNotFound)
		return nil
	}
	b, err := BlobByID(c.Ctx, id)
	if err != nil {
		panic(err)
	}
	if b == nil {
		web.RecordNotFound("Couldn't find ActiveStorage::Blob with 'id'=" + strconv.FormatInt(id, 10))
	}
	return b
}

// requestBase is ActiveStorage::Current.url_options as a URL prefix.
func requestBase(c *web.Context) string {
	// config.assume_ssl: every request is treated as https.
	scheme := "https"
	host := c.R.Host
	if fh := c.HeaderValue("X-Forwarded-Host"); fh != "" {
		host = strings.TrimSpace(strings.Split(fh, ",")[0])
	}
	return scheme + "://" + host
}

// BlobRedirect ports ActiveStorage::Blobs::RedirectController#show.
func BlobRedirect(c *web.Context) {
	b := setBlob(c, "signed_id")
	if b == nil {
		return
	}
	u, err := b.ServiceURL(c.ParamS("disposition"), requestBase(c), time.Now())
	if err != nil {
		panic(err)
	}
	c.Header.Set("Cache-Control", "max-age=300, private")
	c.SetDate()
	c.Redirect(http.StatusFound, u)
}

// BlobProxy ports ActiveStorage::Blobs::ProxyController#show (a Live
// controller: headers go out before the object is read).
func BlobProxy(c *web.Context) {
	b := setBlob(c, "signed_id")
	if b == nil {
		return
	}
	disposition := b.ForcedDispositionForServing()
	if disposition == "" {
		disposition = c.ParamS("disposition")
	}
	if disposition == "" {
		disposition = "inline"
	}
	contentDisposition := ContentDisposition(disposition, SanitizedFilename(b.Filename))
	if rng := c.HeaderValue("Range"); strings.TrimSpace(rng) != "" {
		start, end, ok := singleRange(rng, b.ByteSize)
		if !ok {
			c.HeadFormat(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if streamType(b) == "application/octet-stream" && b.ContentTypeForServing() == "" {
			// send_data(type: nil): ":type option required".
			panic(&web.StandardError{Class: "ArgumentError", Message: ":type option required"})
		}
		data, err := download(c, b)
		if err != nil {
			panic(err)
		}
		if end >= int64(len(data)) {
			end = int64(len(data)) - 1
		}
		chunk := data[min(start, int64(len(data))) : end+1]
		c.Header.Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(b.ByteSize, 10))
		c.Header.Set("Accept-Ranges", "bytes")
		c.Header.Set("Content-Disposition", contentDisposition)
		c.Header.Set("Content-Transfer-Encoding", "binary")
		c.Raw(http.StatusPartialContent, streamType(b), chunk)
		return
	}
	// http_cache_forever: an ETag of the request path and a fixed
	// Last-Modified; the conditional-GET layer answers 304 from them.
	sum := sha256.Sum256([]byte(c.R.URL.RequestURI()))
	c.Header.Set("Etag", `W/"`+hex.EncodeToString(sum[:])[:32]+`"`)
	c.Header.Set("Last-Modified", "Sat, 01 Jan 2011 00:00:00 GMT")
	c.SetDate()
	c.Header.Set("Accept-Ranges", "bytes")
	c.Header.Set("Content-Disposition", contentDisposition)
	data, err := download(c, b)
	if errors.Is(err, s3.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		// expires_now; head :not_found after the headers were committed.
		c.Header.Set("Cache-Control", "no-cache")
		c.Raw(http.StatusNotFound, streamType(b), nil)
		c.KeepRequestIDs = true
		return
	}
	if err != nil {
		panic(err)
	}
	c.Header.Set("Cache-Control", "max-age=3155695200, public, immutable")
	c.Raw(http.StatusOK, streamType(b), data)
}

func dispositionKind(d string) string {
	if d == "attachment" {
		return "attachment"
	}
	return "inline"
}

// singleRange parses one byte range (Rack::Utils.get_byte_ranges).
func singleRange(header string, size int64) (int64, int64, bool) {
	spec, ok := strings.CutPrefix(strings.TrimSpace(header), "bytes=")
	if !ok || strings.Contains(spec, ",") {
		return 0, 0, false
	}
	from, to, ok := strings.Cut(spec, "-")
	if !ok {
		return 0, 0, false
	}
	var start, end int64
	var err error
	switch {
	case from == "":
		n, err := strconv.ParseInt(to, 10, 64)
		if err != nil || n == 0 {
			return 0, 0, false
		}
		start, end = max(size-n, 0), size-1
	default:
		if start, err = strconv.ParseInt(from, 10, 64); err != nil {
			return 0, 0, false
		}
		end = size - 1
		if to != "" {
			if end, err = strconv.ParseInt(to, 10, 64); err != nil || end < start {
				return 0, 0, false
			}
			end = min(end, size-1)
		}
	}
	if start >= size || start > end {
		return 0, 0, false
	}
	return start, end, true
}

func download(c *web.Context, b *Blob) ([]byte, error) {
	if b.ServiceName == "railway_avatars" {
		return s3.FromEnv().Get(c.Ctx, b.Key)
	}
	p, ok := DiskPath(b.ServiceName, b.Key)
	if !ok {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(p)
}

// DiskShow ports ActiveStorage::DiskController#show.
func DiskShow(c *web.Context) {
	v, ok := VerifiedData(c.ParamS("encoded_key"), "blob_key", time.Now())
	key, _ := v.(*rb.Map)
	if !ok || key == nil {
		c.HeadFormat(http.StatusNotFound)
		return
	}
	service := rb.ToS(key.Get("service_name"))
	if _, disk := DiskRoot(service); !disk {
		// named_disk_service falls back to the default (S3) service, which
		// has no path_for: NoMethodError.
		panic(&web.StandardError{Class: "NoMethodError", Message: "undefined method 'path_for' for an instance of ActiveStorage::Service::S3Service"})
	}
	p, ok := DiskPath(service, rb.ToS(key.Get("key")))
	if !ok || !fileExists(p) {
		c.HeadFormat(http.StatusNotFound)
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		c.HeadFormat(http.StatusNotFound)
		return
	}
	st, _ := os.Stat(p)
	ct := rb.ToS(key.Get("content_type"))
	if ct == "" {
		ct = "application/octet-stream"
	}
	disposition := rb.ToS(key.Get("disposition"))
	if disposition == "" {
		disposition = "attachment"
	}
	c.Header.Set("Last-Modified", st.ModTime().UTC().Format(http.TimeFormat))
	c.Header.Set("Accept-Ranges", "bytes")
	c.Header.Set("Content-Disposition", disposition)
	c.Raw(http.StatusOK, ct, data)
}

// DiskUpdate ports ActiveStorage::DiskController#update. Direct uploads go
// to the default (S3) service, so no Disk upload token is ever issued; any
// token that does verify is handled like Rails.
func DiskUpdate(c *web.Context) {
	v, ok := VerifiedData(c.ParamS("encoded_token"), "blob_token", time.Now())
	token, _ := v.(*rb.Map)
	if !ok || token == nil {
		c.HeadBase(http.StatusNotFound)
		return
	}
	c.HeadBase(http.StatusUnprocessableEntity)
}

// Representation ports ActiveStorage::Representations::*Controller#show.
// Blob#representation raises UnrepresentableError (a 500) for a blob that is
// neither variable nor previewable, and a variation key that does not
// verify is a head :not_found. The application never signs a variation key,
// so a request cannot reach the variant itself.
func Representation(c *web.Context) {
	b := setBlob(c, "signed_blob_id")
	if b == nil {
		return
	}
	if !representable(b.ContentTypeString()) {
		panic(&web.StandardError{Class: "ActiveStorage::UnrepresentableError", Message: "Cannot represent blob with content type " + b.ContentTypeString()})
	}
	if _, ok := VerifiedData(c.ParamS("variation_key"), "variation", time.Now()); !ok {
		c.HeadBase(http.StatusNotFound)
		return
	}
	panic(errors.New("image variants are not supported by this deployment"))
}

// variableTypes is config.active_storage.variable_content_types; videos are
// previewable because the production image ships ffmpeg.
var variableTypes = map[string]bool{
	"image/png": true, "image/gif": true, "image/jpeg": true, "image/tiff": true, "image/bmp": true,
	"image/vnd.adobe.photoshop": true, "image/vnd.microsoft.icon": true, "image/webp": true, "image/avif": true,
	"image/heic": true, "image/heif": true,
}

func representable(ct string) bool { return variableTypes[ct] || strings.HasPrefix(ct, "video/") }

// streamType is send_stream/send_data's type: application/octet-stream
// when the blob has none.
func streamType(b *Blob) string {
	if ct := b.ContentTypeForServing(); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
