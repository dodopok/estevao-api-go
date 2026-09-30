package diff

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// The recorded corpus is the oracle's answers, kept so the Go server can be
// checked without a running Rails app: one gzipped JSON-lines file per
// suite, and meta.json. Recorded answers are normalized exactly as the live
// comparison normalizes them. See docs/TESTING.md.

// Meta describes a recording.
type Meta struct {
	// RecordedAt is the instant the database snapshot was taken; replays
	// run the Go server and the harness on a test clock starting there.
	RecordedAt string         `json:"recorded_at"`
	GoCommit   string         `json:"go_commit,omitempty"`
	RailsRef   string         `json:"rails_commit,omitempty"`
	Suites     map[string]int `json:"suites"`
}

// Recorded is one request or scenario of a suite, in suite order.
type Recorded struct {
	Kind     string          `json:"kind"` // "request" or "scenario"
	Index    int             `json:"index"`
	Method   string          `json:"method,omitempty"`
	Path     string          `json:"path,omitempty"`
	Name     string          `json:"name,omitempty"`
	Result   *Result         `json:"result,omitempty"`
	Scenario *ScenarioResult `json:"scenario,omitempty"`
}

// CorpusWriter appends one suite's recordings.
type CorpusWriter struct {
	f  *os.File
	gz *gzip.Writer
	w  *bufio.Writer
	n  int
}

// NewCorpusWriter creates dir/<suite>.jsonl.gz.
func NewCorpusWriter(dir, suite string) (*CorpusWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.Create(filepath.Join(dir, suite+".jsonl.gz"))
	if err != nil {
		return nil, err
	}
	gz, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	return &CorpusWriter{f: f, gz: gz, w: bufio.NewWriter(gz)}, nil
}

// Write appends one entry.
func (c *CorpusWriter) Write(r Recorded) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	c.n++
	if _, err := c.w.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// Close flushes the file and returns how many entries it holds.
func (c *CorpusWriter) Close() (int, error) {
	if err := c.w.Flush(); err != nil {
		return 0, err
	}
	if err := c.gz.Close(); err != nil {
		return 0, err
	}
	return c.n, c.f.Close()
}

// ReadCorpus loads one suite's recordings.
func ReadCorpus(dir, suite string) ([]Recorded, error) {
	f, err := os.Open(filepath.Join(dir, suite+".jsonl.gz"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(gz)
	sc.Buffer(make([]byte, 1<<20), 256<<20)
	var out []Recorded
	for sc.Scan() {
		var r Recorded
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("%s entry %d: %w", suite, len(out)+1, err)
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

// WriteMeta writes dir/meta.json.
func WriteMeta(dir string, m Meta) error {
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(dir, "meta.json"), append(b, '\n'), 0o644)
}

// ReadMeta reads dir/meta.json.
func ReadMeta(dir string) (Meta, error) {
	var m Meta
	b, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

// SameRequest reports whether a recorded request is the one a suite now
// builds at that position (a suite edited after the recording is stale).
func (r Recorded) SameRequest(req Request) bool {
	return r.Kind == "request" && r.Method == methodOf(req) && r.Path == req.Path && r.Name == req.Name
}
