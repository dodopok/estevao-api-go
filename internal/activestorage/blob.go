package activestorage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/s3"
)

// Blob is an active_storage_blobs row.
type Blob struct {
	ID          int64
	Key         string
	Filename    string
	ContentType *string
	Metadata    *string
	ServiceName string
	ByteSize    int64
	Checksum    *string
	CreatedAt   time.Time
}

const blobColumns = `b.id, b.key, b.filename, b.content_type, b.metadata, b.service_name, b.byte_size, b.checksum, b.created_at`

func scanBlob(row interface{ Scan(...any) error }) (*Blob, error) {
	var b Blob
	err := row.Scan(&b.ID, &b.Key, &b.Filename, &b.ContentType, &b.Metadata, &b.ServiceName, &b.ByteSize, &b.Checksum, &b.CreatedAt)
	if db.NoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// BlobByID loads a blob.
func BlobByID(ctx context.Context, id int64) (*Blob, error) {
	return scanBlob(db.Q().QueryRow(ctx, `SELECT `+blobColumns+` FROM active_storage_blobs b WHERE b.id = $1`, id))
}

// Attached returns the blob attached to record under name (has_one_attached).
func Attached(ctx context.Context, recordType string, recordID int64, name string) (*Blob, error) {
	return scanBlob(db.Q().QueryRow(ctx, `SELECT `+blobColumns+` FROM active_storage_attachments a
		JOIN active_storage_blobs b ON b.id = a.blob_id
		WHERE a.record_type = $1 AND a.record_id = $2 AND a.name = $3 ORDER BY a.id LIMIT 1`, recordType, recordID, name))
}

// SignedID ports Blob#signed_id.
func (b *Blob) SignedID() string { return Generate(b.ID, "blob_id") }

// ContentTypeString is content_type (nil as "").
func (b *Blob) ContentTypeString() string { return rb.ToS(rb.Deref(b.ContentType)) }

var servedAsBinary = map[string]bool{
	"text/html": true, "image/svg+xml": true, "application/postscript": true, "application/x-shockwave-flash": true,
	"text/xml": true, "application/xml": true, "application/xhtml+xml": true, "application/mathml+xml": true,
	"text/cache-manifest": true,
}

var allowedInline = map[string]bool{
	"image/webp": true, "image/avif": true, "image/png": true, "image/gif": true, "image/jpeg": true,
	"image/tiff": true, "image/bmp": true, "image/vnd.adobe.photoshop": true, "image/vnd.microsoft.icon": true,
	"application/pdf": true,
}

// ContentTypeForServing ports Blob::Servable#content_type_for_serving.
func (b *Blob) ContentTypeForServing() string {
	if servedAsBinary[b.ContentTypeString()] {
		return "application/octet-stream"
	}
	return b.ContentTypeString()
}

// ForcedDispositionForServing ports forced_disposition_for_serving ("" for nil).
func (b *Blob) ForcedDispositionForServing() string {
	ct := b.ContentTypeString()
	if servedAsBinary[ct] || !allowedInline[ct] {
		return "attachment"
	}
	return ""
}

// SanitizedFilename ports ActiveStorage::Filename#sanitized.
func SanitizedFilename(name string) string {
	name = strings.ToValidUTF8(name, "�")
	name = rb.Strip(name)
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune("‮%$|:;/<>?*\"\t\r\n\\", r) {
			return '-'
		}
		return r
	}, name)
}

// ContentDisposition ports ActionDispatch::Http::ContentDisposition.format.
func ContentDisposition(disposition, filename string) string {
	return disposition + `; filename="` + percentEscape(rb.Transliterate(filename), traditionalSafe) + `"; filename*=UTF-8''` +
		percentEscape(filename, rfc5987Safe)
}

// DispositionWith ports Service#content_disposition_with.
func DispositionWith(kind, filename string) string {
	if kind != "attachment" && kind != "inline" {
		kind = "inline"
	}
	return ContentDisposition(kind, SanitizedFilename(filename))
}

func traditionalSafe(c byte) bool {
	return c == ' ' || isAlnum(c) || strings.IndexByte("!#$+.^_`|~-", c) >= 0
}

func rfc5987Safe(c byte) bool { return isAlnum(c) || strings.IndexByte("!#$&+.^_`|~-", c) >= 0 }

func isAlnum(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
}

func percentEscape(s string, safe func(byte) bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x80 && safe(c) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// escapeSegment ports ActionDispatch::Journey::Router::Utils.escape_segment;
// keepSlash makes it escape_path (for glob segments).
func escapeSegment(s string, keepSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x80 && (isAlnum(c) || strings.IndexByte("-._~!$&'()*+,;=:@", c) >= 0 || keepSlash && c == '/') {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// hostPrefix renders the host option of a URL helper ("http://" is added
// when the host carries no protocol).
func hostPrefix(host string) string {
	host = strings.TrimSuffix(host, "/")
	if !strings.Contains(host, "://") {
		return "http://" + host
	}
	return host
}

// BlobPath ports rails_blob_path(blob): the permanent redirect route.
func BlobPath(b *Blob) string {
	return "/rails/active_storage/blobs/redirect/" + escapeSegment(b.SignedID(), false) + "/" +
		escapeSegment(SanitizedFilename(b.Filename), true)
}

// BlobURL ports rails_blob_url(blob, host:).
func BlobURL(b *Blob, host string) string { return hostPrefix(host) + BlobPath(b) }

// DiskRoot is the root of a Disk service in config/storage.yml, relative to
// the application root (RAILS_ROOT or the working directory).
func DiskRoot(service string) (string, bool) {
	var rel string
	switch service {
	case "production":
		rel = "public/storage"
	case "local":
		rel = "storage"
	case "test":
		rel = "tmp/storage"
	default:
		return "", false
	}
	root := config.PresenceOr("RAILS_ROOT", ".")
	return filepath.Join(root, rel), true
}

// ServiceURL ports Blob#url(disposition:) on the blob's own service.
// requestBase is the scheme://host of the current request
// (ActiveStorage::Current.url_options), used by Disk services.
func (b *Blob) ServiceURL(disposition, requestBase string, now time.Time) (string, error) {
	if d := b.ForcedDispositionForServing(); d != "" {
		disposition = d
	}
	if b.ServiceName == "railway_avatars" {
		expiresIn := 300
		return s3.FromEnv().PresignedGet(b.Key, expiresIn, DispositionWith(disposition, b.Filename), b.ContentTypeForServing(), now), nil
	}
	if _, ok := DiskRoot(b.ServiceName); ok {
		var exp *time.Time
		if b.ServiceName != "production" { // the production Disk service is public
			t := now.Add(5 * time.Minute)
			exp = &t
		}
		var ct any
		if b.ContentType != nil || servedAsBinary[b.ContentTypeString()] {
			ct = b.ContentTypeForServing()
		}
		token := GenerateData(rb.M("key", b.Key, "disposition", DispositionWith(disposition, b.Filename),
			"content_type", ct, "service_name", b.ServiceName), "blob_key", exp)
		return requestBase + "/rails/active_storage/disk/" + escapeSegment(token, false) + "/" +
			escapeSegment(SanitizedFilename(b.Filename), true), nil
	}
	return "", fmt.Errorf("Missing configuration for the %s Active Storage service", b.ServiceName)
}

// DiskPath ports DiskService#path_for with its traversal checks.
func DiskPath(service, key string) (string, bool) {
	root, ok := DiskRoot(service)
	if !ok || strings.TrimSpace(key) == "" || strings.ContainsRune(key, 0) {
		return "", false
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "." || seg == ".." {
			return "", false
		}
	}
	folder := substr(key, 0, 2) + "/" + substr(key, 2, 4)
	absRoot, _ := filepath.Abs(root)
	p, err := filepath.Abs(filepath.Join(root, folder, key))
	if err != nil || !strings.HasPrefix(p, absRoot+"/") {
		return "", false
	}
	return p, true
}

func substr(s string, from, to int) string {
	if from >= len(s) {
		return ""
	}
	return s[from:min(to, len(s))]
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}
