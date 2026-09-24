package audio

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/rb"
)

// Storage ports Audio::Storage for reading: where a clip lives and the URL the
// app is given for it. Production (or AUDIO_STORAGE_SERVICE) serves presigned
// S3 URLs through the `railway_avatars` Active Storage service; other
// environments serve the public/audio path.
type Storage struct {
	provider *Provider
	remote   bool
	// now is the signing clock (time.Now outside tests).
	now func() time.Time
}

// NewStorage ports Audio::Storage.new(provider).
func NewStorage(p *Provider) *Storage {
	return &Storage{provider: p, remote: config.RailsEnv() == "production" || !rb.BlankString(os.Getenv("AUDIO_STORAGE_SERVICE")), now: time.Now}
}

// FilenameFor ports filename_for(key).
func (s *Storage) FilenameFor(key string) string {
	return "tts/" + s.provider.Name + "/" + key + ".mp3"
}

// URLFor ports url_for(filename).
func (s *Storage) URLFor(filename string) string {
	if !s.remote {
		return "/audio/" + filename
	}
	return s.service().presignedGet("audio/"+filename, urlExpiresIn(), path.Base(filename), s.now())
}

func urlExpiresIn() int {
	raw, ok := os.LookupEnv("AUDIO_URL_EXPIRES_IN")
	if !ok {
		return 3600
	}
	n, ok := rubyInteger(raw)
	if !ok {
		return 3600
	}
	return min(max(n, 300), 86400)
}

// DurationFor ports duration_for(filename): the playing time read from the
// stored object itself, nil when it cannot be read.
func (s *Storage) DurationFor(filename string) *float64 {
	var data []byte
	if !s.remote {
		p := filepath.Join(config.PublicDir(), "audio", filename)
		st, err := os.Stat(p)
		if err != nil || !st.Mode().IsRegular() || st.Size() > mp3MaxBytes {
			return nil
		}
		if data, err = os.ReadFile(p); err != nil {
			return nil
		}
	} else {
		u := s.service().presignedGet("audio/"+filename, 300, path.Base(filename), s.now())
		resp, err := http.Get(u)
		if err != nil {
			return nil
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil
		}
		if data, err = io.ReadAll(io.LimitReader(resp.Body, mp3MaxBytes+1)); err != nil || len(data) > mp3MaxBytes {
			return nil
		}
	}
	return Mp3Duration(data)
}

// s3Service is the S3 Active Storage service named by AUDIO_STORAGE_SERVICE
// (default railway_avatars, config/storage.yml).
type s3Service struct {
	accessKey, secretKey, region, bucket, endpoint string
	forcePathStyle                                 bool
}

func (s *Storage) service() *s3Service {
	name := os.Getenv("AUDIO_STORAGE_SERVICE")
	if rb.BlankString(name) {
		name = "railway_avatars"
	}
	if name != "railway_avatars" {
		// The other services in config/storage.yml are Disk services that
		// this port does not serve audio from.
		panic(&rb.RubyError{Class: "ArgumentError", Message: "Active Storage service not configured: " + name})
	}
	region := os.Getenv("AVATAR_BUCKET_REGION")
	if _, ok := os.LookupEnv("AVATAR_BUCKET_REGION"); !ok {
		region = "auto"
	}
	return &s3Service{
		accessKey: os.Getenv("AVATAR_BUCKET_ACCESS_KEY_ID"), secretKey: os.Getenv("AVATAR_BUCKET_SECRET_ACCESS_KEY"),
		region: region, bucket: os.Getenv("AVATAR_BUCKET_NAME"), endpoint: os.Getenv("AVATAR_BUCKET_ENDPOINT"),
	}
}

var virtualHostableBucket = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

// presignedGet reproduces ActiveStorage::Service::S3Service#url for a private
// file: aws-sdk-s3's presigned GET with the inline disposition and the
// audio/mpeg content type, signed with SigV4 query parameters.
func (s *s3Service) presignedGet(key string, expiresIn int, filename string, now time.Time) string {
	endpoint := s.endpoint
	if endpoint == "" {
		endpoint = "https://s3." + s.region + ".amazonaws.com"
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		panic(err)
	}
	host := u.Host
	basePath := strings.TrimSuffix(u.Path, "/")
	escapedKey := escapePath(key)
	var canonicalURI string
	if s.forcePathStyle || net.ParseIP(u.Hostname()) != nil || !virtualHostableBucket.MatchString(s.bucket) {
		canonicalURI = basePath + "/" + awsEscape(s.bucket) + "/" + escapedKey
	} else {
		host = s.bucket + "." + host
		canonicalURI = basePath + "/" + escapedKey
	}
	amzDate := now.UTC().Format("20060102T150405Z")
	day := amzDate[:8]
	scope := day + "/" + s.region + "/s3/aws4_request"
	disposition := `inline; filename="` + filename + `"; filename*=UTF-8''` + awsEscape(filename)
	params := [][2]string{
		{"response-content-disposition", disposition},
		{"response-content-type", "audio/mpeg"},
		{"X-Amz-Algorithm", "AWS4-HMAC-SHA256"},
		{"X-Amz-Credential", s.accessKey + "/" + scope},
		{"X-Amz-Date", amzDate},
		{"X-Amz-Expires", fmt.Sprint(expiresIn)},
		{"X-Amz-SignedHeaders", "host"},
	}
	query := encodeParams(params)
	sorted := append([][2]string(nil), params...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i][0] < sorted[j][0] })
	canonicalRequest := strings.Join([]string{
		"GET", canonicalURI, encodeParams(sorted), "host:" + host + "\n", "host", "UNSIGNED-PAYLOAD",
	}, "\n")
	digest := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(digest[:])
	k := hmacSHA256([]byte("AWS4"+s.secretKey), day)
	k = hmacSHA256(k, s.region)
	k = hmacSHA256(k, "s3")
	k = hmacSHA256(k, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(k, stringToSign))
	return u.Scheme + "://" + host + canonicalURI + "?" + query + "&X-Amz-Signature=" + signature
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func encodeParams(params [][2]string) string {
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = awsEscape(p[0]) + "=" + awsEscape(p[1])
	}
	return strings.Join(parts, "&")
}

// awsEscape is Seahorse::Util.uri_escape: everything but A-Z a-z 0-9 - _ . ~
// percent-encoded.
func awsEscape(s string) string {
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
		parts[i] = awsEscape(part)
	}
	return strings.Join(parts, "/")
}
