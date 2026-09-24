package activestorage

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/s3"
)

// Upload is the attachable (an uploaded file).
type Upload struct {
	Filename    string
	ContentType *string
	Data        []byte
}

// IdentifyContentType ports Marcel::MimeType.for(io, name:, declared_type:)
// for the formats an upload can realistically carry: the magic-number type
// wins unless the declared type (or else the extension) is of the same root.
func IdentifyContentType(data []byte, filename string, declared *string) string {
	fallback := ""
	if declared != nil {
		t := strings.ToLower(strings.TrimSpace(strings.SplitN(*declared, ";", 2)[0]))
		if t != "" && t != "application/octet-stream" {
			fallback = t
		}
	}
	if fallback == "" {
		fallback = typeForName(filename)
	}
	if fallback == "" {
		fallback = "application/octet-stream"
	}
	magic := sniff(data)
	if magic == "" {
		return fallback
	}
	if rootType(magic) == rootType(fallback) {
		return fallback
	}
	return magic
}

func rootType(t string) string {
	switch t {
	case "image/pjpeg", "image/jpg":
		return "image/jpeg"
	case "image/x-png":
		return "image/png"
	}
	return t
}

func sniff(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(b, []byte("GIF87a")), bytes.HasPrefix(b, []byte("GIF89a")):
		return "image/gif"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp"
	case bytes.HasPrefix(b, []byte("BM")) && len(b) > 14:
		return "image/bmp"
	case bytes.HasPrefix(b, []byte("II*\x00")), bytes.HasPrefix(b, []byte("MM\x00*")):
		return "image/tiff"
	case bytes.HasPrefix(b, []byte("%PDF-")):
		return "application/pdf"
	case len(b) >= 12 && string(b[4:8]) == "ftyp" && (string(b[8:12]) == "avif" || string(b[8:12]) == "avis"):
		return "image/avif"
	case len(b) >= 12 && string(b[4:8]) == "ftyp" && (string(b[8:12]) == "heic" || string(b[8:12]) == "heix"):
		return "image/heic"
	}
	return ""
}

func typeForName(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return ""
	}
	switch strings.ToLower(name[i+1:]) {
	case "jpg", "jpeg", "jpe":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "bmp":
		return "image/bmp"
	case "tif", "tiff":
		return "image/tiff"
	case "svg":
		return "image/svg+xml"
	case "pdf":
		return "application/pdf"
	case "txt":
		return "text/plain"
	case "heic":
		return "image/heic"
	case "avif":
		return "image/avif"
	}
	return ""
}

// Attach ports `record.<name>.attach(upload)` for has_one_attached with the
// default dependent: :purge_later and touch_attachment_records. The blob and
// the attachment are written (and the record touched) in one transaction; the
// bytes are uploaded after commit; the replaced blob is purged and the new
// one analyzed off the request, as their jobs would.
func Attach(ctx context.Context, recordType, table string, recordID int64, name string, up Upload, now time.Time) (*Blob, error) {
	sum := md5.Sum(up.Data)
	checksum := base64.StdEncoding.EncodeToString(sum[:])
	ct := IdentifyContentType(up.Data, up.Filename, up.ContentType)
	meta := `{"identified":true}`
	b := &Blob{Key: GenerateKey(), Filename: up.Filename, ContentType: &ct, Metadata: &meta, ServiceName: DefaultService,
		ByteSize: int64(len(up.Data)), Checksum: &checksum}
	now = now.UTC().Truncate(time.Microsecond)
	var replaced []int64
	err := db.InTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO active_storage_blobs (key, filename, content_type, metadata, service_name, byte_size, checksum, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id, created_at`,
			b.Key, b.Filename, b.ContentType, b.Metadata, b.ServiceName, b.ByteSize, b.Checksum, now).Scan(&b.ID, &b.CreatedAt); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `DELETE FROM active_storage_attachments WHERE record_type = $1 AND record_id = $2 AND name = $3 RETURNING blob_id`,
			recordType, recordID, name)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			replaced = append(replaced, id)
		}
		rows.Close()
		if _, err := tx.Exec(ctx, `INSERT INTO active_storage_attachments (name, record_type, record_id, blob_id, created_at)
			VALUES ($1, $2, $3, $4, $5)`, name, recordType, recordID, b.ID, now); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE `+table+` SET updated_at = $1 WHERE id = $2`, now, recordID)
		return err
	})
	if err != nil {
		return nil, err
	}
	for _, id := range replaced {
		enqueue(func(ctx context.Context) error { return PurgeBlob(ctx, id) })
	}
	enqueue(func(ctx context.Context) error { return Analyze(ctx, b.ID) })
	if err := upload(ctx, b, up.Data); err != nil {
		return nil, err
	}
	return b, nil
}

// upload ports S3Service#upload with the blob's service_metadata.
func upload(ctx context.Context, b *Blob, data []byte) error {
	disposition := ""
	contentType := b.ContentTypeString()
	if servedAsBinary[contentType] {
		contentType = "application/octet-stream"
		disposition = DispositionWith("attachment", b.Filename)
	} else if !allowedInline[contentType] {
		disposition = DispositionWith("attachment", b.Filename)
	}
	return s3.FromEnv().Put(ctx, b.Key, data, rb64(b.Checksum), contentType, disposition)
}

func rb64(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Purge ports `record.<name>.purge`: the attachment row goes (touching the
// record), then the blob row and its stored object.
func Purge(ctx context.Context, recordType, table string, recordID int64, name string, now time.Time) error {
	var blobID int64
	err := db.InTx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `DELETE FROM active_storage_attachments WHERE id = (SELECT id FROM active_storage_attachments
			WHERE record_type = $1 AND record_id = $2 AND name = $3 ORDER BY id LIMIT 1) RETURNING blob_id`,
			recordType, recordID, name).Scan(&blobID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE `+table+` SET updated_at = $1 WHERE id = $2`, now.UTC().Truncate(time.Microsecond), recordID)
		return err
	})
	if err != nil {
		return err
	}
	return PurgeBlob(ctx, blobID)
}

// PurgeBlob ports Blob#purge (and ActiveStorage::PurgeJob): the row, its
// variant records, then the object and, for an image, its variants.
func PurgeBlob(ctx context.Context, id int64) error {
	b, err := BlobByID(ctx, id)
	if err != nil || b == nil {
		return err
	}
	var attached bool
	if err := db.Q().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM active_storage_attachments WHERE blob_id = $1)`, id).Scan(&attached); err != nil {
		return err
	}
	if attached {
		return nil // Blob#purge refuses (the dependent-purge job checks this too)
	}
	if _, err := db.Q().Exec(ctx, `DELETE FROM active_storage_variant_records WHERE blob_id = $1`, id); err != nil {
		return err
	}
	if _, err := db.Q().Exec(ctx, `DELETE FROM active_storage_blobs WHERE id = $1`, id); err != nil {
		return err
	}
	if b.ServiceName != DefaultService {
		if p, ok := DiskPath(b.ServiceName, b.Key); ok {
			_ = removeFile(p)
		}
		return nil
	}
	client := s3.FromEnv()
	if err := client.Delete(ctx, b.Key); err != nil {
		return err
	}
	if strings.HasPrefix(b.ContentTypeString(), "image") {
		return client.DeletePrefixed(ctx, "variants/"+b.Key+"/")
	}
	return nil
}

// enqueue runs a job off the request (see docs/EQUIVALENCE.md: the Go
// service has no durable queue for these Active Storage jobs).
func enqueue(job func(ctx context.Context) error) {
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Warn("active storage job failed", "error", rec)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := job(ctx); err != nil {
			slog.Warn("active storage job failed", "error", err)
		}
	}()
}

// PurgeLater ports blob.purge_later (ActiveStorage::PurgeJob), off the request.
func PurgeLater(id int64) {
	enqueue(func(ctx context.Context) error { return PurgeBlob(ctx, id) })
}
