package activestorage

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	_ "image/gif" // decoders for image.DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/dodopok/estevao-api-go/internal/db"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/s3"
)

func removeFile(p string) error { return os.Remove(p) }

// Analyze ports ActiveStorage::AnalyzeJob with the image analyzer: width
// and height (after EXIF rotation, as vips autorot reports them) are merged
// into the metadata with analyzed: true. A file that cannot be read gets
// only analyzed: true, like the null analyzer.
func Analyze(ctx context.Context, id int64) error {
	b, err := BlobByID(ctx, id)
	if err != nil || b == nil {
		return err
	}
	meta := rb.NewMap()
	if b.Metadata != nil {
		if v, err := rb.ParseJSON([]byte(*b.Metadata)); err == nil {
			if m, ok := v.(*rb.Map); ok {
				meta = m
			}
		}
	}
	if isImage(b.ContentTypeString()) {
		if data, err := s3.FromEnv().Get(ctx, b.Key); err == nil {
			if w, h, ok := ImageSize(data); ok {
				meta.Set("width", w)
				meta.Set("height", h)
			}
		}
	}
	meta.Set("analyzed", true)
	_, err = db.Q().Exec(ctx, `UPDATE active_storage_blobs SET metadata = $1 WHERE id = $2`, string(rb.JSON(meta)), id)
	return err
}

func isImage(ct string) bool { return len(ct) > 6 && ct[:6] == "image/" }

// ImageSize reads an image's display dimensions (JPEG, PNG, GIF, WebP).
func ImageSize(data []byte) (int, int, bool) {
	if w, h, ok := webpSize(data); ok {
		return w, h, true
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, false
	}
	if o := jpegOrientation(data); o >= 5 && o <= 8 {
		return cfg.Height, cfg.Width, true
	}
	return cfg.Width, cfg.Height, true
}

func webpSize(b []byte) (int, int, bool) {
	if len(b) < 30 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return 0, 0, false
	}
	switch string(b[12:16]) {
	case "VP8 ":
		if b[23] != 0x9d || b[24] != 0x01 || b[25] != 0x2a {
			return 0, 0, false
		}
		return int(binary.LittleEndian.Uint16(b[26:28]) & 0x3fff), int(binary.LittleEndian.Uint16(b[28:30]) & 0x3fff), true
	case "VP8L":
		if b[20] != 0x2f {
			return 0, 0, false
		}
		bits := binary.LittleEndian.Uint32(b[21:25])
		return int(bits&0x3fff) + 1, int((bits>>14)&0x3fff) + 1, true
	case "VP8X":
		w := int(b[24]) | int(b[25])<<8 | int(b[26])<<16
		h := int(b[27]) | int(b[28])<<8 | int(b[29])<<16
		return w + 1, h + 1, true
	}
	return 0, 0, false
}

// jpegOrientation returns the EXIF orientation tag of a JPEG (0 if none).
func jpegOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 0
	}
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return 0
		}
		marker := b[i+1]
		size := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		if marker == 0xDA || size < 2 || i+2+size > len(b) {
			return 0
		}
		seg := b[i+4 : i+2+size]
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return exifOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 0
}

func exifOrientation(t []byte) int {
	if len(t) < 8 {
		return 0
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}
	off := int(bo.Uint32(t[4:8]))
	if off+2 > len(t) {
		return 0
	}
	n := int(bo.Uint16(t[off : off+2]))
	for k := 0; k < n; k++ {
		e := off + 2 + 12*k
		if e+12 > len(t) {
			return 0
		}
		if bo.Uint16(t[e:e+2]) == 0x0112 {
			return int(bo.Uint16(t[e+8 : e+10]))
		}
	}
	return 0
}
