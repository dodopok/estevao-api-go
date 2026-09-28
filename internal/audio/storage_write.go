package audio

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path"
	"path/filepath"

	"github.com/dodopok/estevao-api-go/internal/activestorage"
	"github.com/dodopok/estevao-api-go/internal/config"
	"github.com/dodopok/estevao-api-go/internal/rb"
	"github.com/dodopok/estevao-api-go/internal/s3"
)

// URLExpiresIn ports url_expires_in.
func URLExpiresIn() int { return urlExpiresIn() }

// PlainStorage ports Audio::Storage.new with no provider (enough to address,
// serve and delete an existing filename).
func PlainStorage() *Storage { return NewStorage(nil) }

func (s *Storage) checkService() {
	if s.remote {
		s.service() // raises ArgumentError for an unconfigured service
	}
}

// CandidateFilenameFor ports candidate_filename_for(key).
func (s *Storage) CandidateFilenameFor(key string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "tts/" + s.provider.Name + "/candidates/" + key + "-" + hex.EncodeToString(b[:]) + ".mp3"
}

func localPath(filename string) string {
	return filepath.Join(config.PublicDir(), "audio", filename)
}

// Exists ports exists?(filename).
func (s *Storage) Exists(ctx context.Context, filename string) bool {
	if s.remote {
		s.checkService()
		_, err := s3.FromEnv().Get(ctx, "audio/"+filename)
		return err == nil
	}
	st, err := os.Stat(localPath(filename))
	return err == nil && st.Mode().IsRegular() && st.Size() > 0
}

// Store ports store(filename, source): an atomic local write, or an upload
// under audio/ with the inline disposition and audio/mpeg type.
func (s *Storage) Store(ctx context.Context, filename string, data []byte) error {
	if s.remote {
		s.checkService()
		sum := md5.Sum(data)
		return s3.FromEnv().Put(ctx, "audio/"+filename, data, base64.StdEncoding.EncodeToString(sum[:]), "audio/mpeg",
			activestorage.DispositionWith("inline", path.Base(filename)))
	}
	dest := localPath(filename)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	var b [6]byte
	_, _ = rand.Read(b[:])
	tmp := dest + "." + hex.EncodeToString(b[:]) + ".part"
	defer os.Remove(tmp)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// Read returns a stored object's bytes.
func (s *Storage) Read(ctx context.Context, filename string) ([]byte, error) {
	if s.remote {
		s.checkService()
		return s3.FromEnv().Get(ctx, "audio/"+filename)
	}
	return os.ReadFile(localPath(filename))
}

// Copy ports copy(source, destination).
func (s *Storage) Copy(ctx context.Context, source, destination string) error {
	data, err := s.Read(ctx, source)
	if err != nil {
		return &rb.RubyError{Class: "ActiveStorage::FileNotFoundError", Message: err.Error()}
	}
	return s.Store(ctx, destination, data)
}

// Delete ports delete(filename): idempotent on both backends.
func (s *Storage) Delete(ctx context.Context, filename string) error {
	if s.remote {
		s.checkService()
		return s3.FromEnv().Delete(ctx, "audio/"+filename)
	}
	err := os.Remove(localPath(filename))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
