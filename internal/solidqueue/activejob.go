// Package solidqueue runs background jobs on the Solid Queue tables the
// Rails application uses (solid_queue_*), with ActiveJob's serialization,
// so a job enqueued by either stack can be performed by the other: jobs,
// ready/scheduled/claimed/failed executions, processes and recurring tasks
// are written as Solid Queue 1.3 writes them.
package solidqueue

import (
	"crypto/rand"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/rb"
)

// ActiveJob serialization markers (ActiveJob::Arguments).
const (
	symbolKeysKey   = "_aj_symbol_keys"
	ruby2KeywordKey = "_aj_ruby2_keywords"
	serializedKey   = "_aj_serialized"
	indifferentKey  = "_aj_hash_with_indifferent_access"
	globalIDKey     = "_aj_globalid"
)

// Kwargs serializes keyword arguments (a ruby2_keywords hash): every key is
// a symbol. Pairs are name, value; values must already be serialized.
func Kwargs(pairs ...any) *rb.Map {
	m := rb.NewMap()
	var keys []any
	for i := 0; i < len(pairs); i += 2 {
		k := pairs[i].(string)
		m.Set(k, pairs[i+1])
		keys = append(keys, k)
	}
	if keys == nil {
		keys = []any{}
	}
	m.Set(ruby2KeywordKey, keys)
	return m
}

// StringHash serializes a Hash whose keys are all strings.
func StringHash(m *rb.Map) *rb.Map {
	out := rb.NewMap()
	m.Each(func(k string, v any) { out.Set(k, SerializeValue(v)) })
	out.Set(symbolKeysKey, []any{})
	return out
}

// SymbolHash serializes a Hash whose keys are all symbols.
func SymbolHash(m *rb.Map) *rb.Map {
	out := rb.NewMap()
	keys := []any{}
	m.Each(func(k string, v any) {
		out.Set(k, SerializeValue(v))
		keys = append(keys, k)
	})
	out.Set(symbolKeysKey, keys)
	return out
}

// IndifferentHash serializes a HashWithIndifferentAccess (and so an
// ActionController::Parameters): nested hashes are indifferent too.
func IndifferentHash(m *rb.Map) *rb.Map {
	out := rb.NewMap()
	m.Each(func(k string, v any) { out.Set(k, serializeIndifferent(v)) })
	out.Set(indifferentKey, true)
	return out
}

func serializeIndifferent(v any) any {
	switch x := v.(type) {
	case *rb.Map:
		return IndifferentHash(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = serializeIndifferent(e)
		}
		return out
	}
	return v
}

// Symbol serializes a Ruby Symbol.
func Symbol(name string) *rb.Map {
	return rb.M(serializedKey, "ActiveJob::Serializers::SymbolSerializer", "value", name)
}

// SerializeValue serializes a JSON-shaped value the way ActiveJob does for
// the same Ruby value built from JSON (string-keyed hashes).
func SerializeValue(v any) any {
	switch x := v.(type) {
	case *rb.Map:
		return StringHash(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = SerializeValue(e)
		}
		return out
	}
	return v
}

// Deserialize ports ActiveJob::Arguments.deserialize for the shapes this
// application enqueues: symbol/keyword markers are dropped (keys become
// plain strings) and the Symbol serializer yields its name.
func Deserialize(v any) (any, error) {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			d, err := Deserialize(e)
			if err != nil {
				return nil, err
			}
			out[i] = d
		}
		return out, nil
	case *rb.Map:
		if s, ok := x.Lookup(serializedKey); ok {
			if s == "ActiveJob::Serializers::SymbolSerializer" {
				return rb.ToS(x.Get("value")), nil
			}
			return nil, &Error{Class: "ActiveJob::DeserializationError", Message: fmt.Sprintf("unsupported serializer %v", s)}
		}
		if g, ok := x.Lookup(globalIDKey); ok {
			return GlobalID(rb.ToS(g)), nil
		}
		out := rb.NewMap()
		var err error
		x.Each(func(k string, e any) {
			if err != nil || k == symbolKeysKey || k == ruby2KeywordKey || k == indifferentKey {
				return
			}
			var d any
			d, err = Deserialize(e)
			out.Set(k, d)
		})
		if err != nil {
			return nil, err
		}
		return out, nil
	}
	return v, nil
}

// GlobalID is a deserialized GlobalID argument (its URI); the job locates
// the record itself.
type GlobalID string

// GlobalIDApp is GlobalID.app for this application.
const GlobalIDApp = "estevao-api"

// RecordGID serializes a record reference ("gid://estevao-api/Model/id").
func RecordGID(model string, id int64) *rb.Map {
	return rb.M(globalIDKey, "gid://"+GlobalIDApp+"/"+model+"/"+strconv.FormatInt(id, 10))
}

// Locate parses a GlobalID of model, returning its id.
func (g GlobalID) Locate(model string) (int64, bool) {
	prefix := "gid://" + GlobalIDApp + "/" + model + "/"
	rest, ok := strings.CutPrefix(string(g), prefix)
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	return id, err == nil
}

// IsKwargs reports whether a serialized argument is a keyword hash.
func IsKwargs(v any) bool {
	m, ok := v.(*rb.Map)
	return ok && m.Has(ruby2KeywordKey)
}

// NewJobID is SecureRandom.uuid.
func NewJobID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// iso9 is Time#utc.iso8601(9).
func iso9(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000Z") }

// Job is an ActiveJob instance about to be enqueued.
type Job struct {
	Class     string
	Queue     string
	Priority  *int
	Arguments []any // ActiveJob-serialized
	// ScheduledAt is when the job becomes due (zero: now).
	ScheduledAt time.Time

	JobID               string
	Executions          int
	ExceptionExecutions *rb.Map
	Locale              string
	Timezone            string
}

// Serialize ports ActiveJob::Core#serialize.
func (j *Job) Serialize(now time.Time) *rb.Map {
	var priority any
	if j.Priority != nil {
		priority = *j.Priority
	}
	ee := j.ExceptionExecutions
	if ee == nil {
		ee = rb.NewMap()
	}
	args := j.Arguments
	if args == nil {
		args = []any{}
	}
	var scheduled any
	if !j.ScheduledAt.IsZero() {
		scheduled = iso9(j.ScheduledAt)
	}
	return rb.M(
		"job_class", j.Class,
		"job_id", j.JobID,
		"provider_job_id", nil,
		"queue_name", j.Queue,
		"priority", priority,
		"arguments", args,
		"executions", j.Executions,
		"exception_executions", ee,
		"locale", j.Locale,
		"timezone", j.Timezone,
		"enqueued_at", iso9(now),
		"scheduled_at", scheduled,
	)
}

func (j *Job) defaults(now time.Time) {
	if j.JobID == "" {
		j.JobID = NewJobID()
	}
	if j.Queue == "" {
		j.Queue = "default"
	}
	if j.Locale == "" {
		j.Locale = "en"
	}
	if j.Timezone == "" {
		j.Timezone = "America/Sao_Paulo"
	}
	if j.ScheduledAt.IsZero() {
		j.ScheduledAt = now
	}
}

// sortedKeys is a helper for deterministic iteration in logs and tests.
func sortedKeys(m map[string]Handler) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
