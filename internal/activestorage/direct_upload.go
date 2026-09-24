package activestorage

import (
	"crypto/rand"
	"math/big"
	"time"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/s3"
	"github.com/dodopok/estevao-api-go/internal/web"
)

// DefaultService is config.active_storage.service in production.
const DefaultService = "railway_avatars"

const base36 = "0123456789abcdefghijklmnopqrstuvwxyz"

// GenerateKey ports ActiveStorage::Blob.generate_unique_secure_token
// (SecureRandom.base36(28)).
func GenerateKey() string {
	b := make([]byte, 28)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(36))
		if err != nil {
			panic(err)
		}
		b[i] = base36[n.Int64()]
	}
	return string(b)
}

// DirectUploadsCreate ports ActiveStorage::DirectUploadsController#create.
// Like the Rails engine route, it needs no authentication.
func DirectUploadsCreate(c *web.Context) {
	blob, _ := c.Param("blob").(*rb.Map)
	if blob == nil || blob.Len() == 0 {
		panic(&web.StandardError{Class: "ActionController::ParameterMissing", Message: "param is missing or the value is empty or invalid: blob"})
	}
	scalar := func(k string) (any, bool) {
		v, ok := blob.Lookup(k)
		switch v.(type) {
		case *rb.Map, []any:
			return nil, false // not a permitted scalar
		}
		return v, ok
	}
	filename, hasFilename := scalar("filename")
	byteSize, hasSize := scalar("byte_size")
	checksum, hasChecksum := scalar("checksum")
	for _, missing := range []struct {
		ok   bool
		name string
	}{{hasFilename, "filename"}, {hasSize, "byte_size"}, {hasChecksum, "checksum"}} {
		if !missing.ok {
			panic(&web.StandardError{Class: "ArgumentError", Message: "missing keyword: :" + missing.name})
		}
	}
	contentType, _ := scalar("content_type")
	metadata := rb.NewMap()
	if m, ok := blob.Get("metadata").(*rb.Map); ok {
		metadata = m
	}
	size, sizeOK := castInteger(byteSize)
	if rb.Blank(checksum) {
		panic(&web.StandardError{Class: "ActiveRecord::RecordInvalid", Message: "Validation failed: Checksum can't be blank"})
	}
	if !sizeOK {
		panic(&web.StandardError{Class: "ActiveRecord::NotNullViolation", Message: "null value in column \"byte_size\""})
	}
	b := &Blob{Key: GenerateKey(), Filename: rb.ToS(filename), ServiceName: DefaultService, ByteSize: size}
	if contentType != nil {
		s := rb.ToS(contentType)
		b.ContentType = &s
	}
	cs := rb.ToS(checksum)
	b.Checksum = &cs
	if metadata.Len() > 0 { // the store coder writes NULL for an empty hash
		meta := string(rb.JSON(metadata))
		b.Metadata = &meta
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	err := db.Q().QueryRow(c.Ctx, `INSERT INTO active_storage_blobs (key, filename, content_type, metadata, service_name, byte_size, checksum, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id, created_at`,
		b.Key, b.Filename, b.ContentType, b.Metadata, b.ServiceName, b.ByteSize, b.Checksum, now).Scan(&b.ID, &b.CreatedAt)
	if err != nil {
		panic(err)
	}
	out, err := b.AsJSON(c)
	if err != nil {
		panic(err)
	}
	var ct any
	if b.ContentType != nil {
		ct = *b.ContentType
	}
	client := s3.FromEnv()
	out.Set("direct_upload", rb.M(
		"url", client.PresignedPut(b.Key, 300, rb.ToS(ct), b.ByteSize, cs, time.Now()),
		"headers", rb.M("Content-Type", ct, "Content-MD5", cs, "Content-Disposition", DispositionWith("", b.Filename)),
	))
	c.JSON(200, out)
}

// castInteger ports ActiveModel::Type::Integer#cast for a parameter.
func castInteger(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int64:
		return x, true
	case float64:
		return int64(x), true
	case bool:
		if x {
			return 1, true
		}
		return 0, true
	case string:
		n, ok := leadingInteger(x)
		return n, ok
	}
	return 0, false
}

// leadingInteger ports String#to_i guarded by ActiveModel's numeric check.
func leadingInteger(s string) (int64, bool) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n') {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	if i >= len(s) || s[i] < '0' || s[i] > '9' {
		return 0, false
	}
	var n int64
	for ; i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '_'); i++ {
		if s[i] != '_' {
			n = n*10 + int64(s[i]-'0')
		}
	}
	if neg {
		n = -n
	}
	return n, true
}

// AsJSON ports blob.as_json(methods: :signed_id): the columns in table
// order, then attachable_sgid and signed_id.
func (b *Blob) AsJSON(c *web.Context) (*rb.Map, error) {
	cols, err := db.Columns(c.Ctx, "active_storage_blobs")
	if err != nil {
		return nil, err
	}
	out := rb.NewMap()
	for _, col := range cols {
		switch col {
		case "id":
			out.Set("id", b.ID)
		case "key":
			out.Set("key", b.Key)
		case "filename":
			out.Set("filename", b.Filename)
		case "content_type":
			out.Set("content_type", rb.Deref(b.ContentType))
		case "metadata":
			var meta any = rb.NewMap()
			if b.Metadata != nil {
				if v, err := rb.ParseJSON([]byte(*b.Metadata)); err == nil {
					meta = v
				}
			}
			out.Set("metadata", meta)
		case "service_name":
			out.Set("service_name", b.ServiceName)
		case "byte_size":
			out.Set("byte_size", b.ByteSize)
		case "checksum":
			out.Set("checksum", rb.Deref(b.Checksum))
		case "created_at":
			out.Set("created_at", rb.FormatTime(b.CreatedAt))
		}
	}
	out.Set("attachable_sgid", AttachableSGID(b.ID))
	out.Set("signed_id", b.SignedID())
	return out, nil
}
