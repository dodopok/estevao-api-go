package diff

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

// Scenario is a sequence of requests with persisted effects. Each side runs
// it from the same starting state: Setup is applied, the id sequences of
// Tables are rewound to their current maximum (so both sides create the same
// ids) and the side's cache is flushed. After the steps, every Snapshot
// query is run; its rows are part of the comparison.
type Scenario struct {
	Name  string
	Setup string
	// Prepare runs before each side (after Setup), e.g. to reset the fake S3.
	Prepare  func() error
	Tables   []string
	Steps    []Request
	Snapshot []string
	// HTTPSnapshots are URLs fetched after the steps (e.g. the fake S3
	// listing); their bodies are compared.
	HTTPSnapshots []string
	// Settle runs after the steps and before the snapshot (e.g. performs
	// the jobs the oracle enqueued).
	Settle func(side Side) error
}

// Side is one server under test.
type Side struct {
	Name     string // "rails" or "go"
	BaseURL  string
	RedisURL string
}

// ScenarioResult is one side's run.
type ScenarioResult struct {
	Steps     []*Result
	Snapshots [][]string
}

// RunScenario runs s against one side.
func RunScenario(ctx context.Context, s Scenario, side Side) (*ScenarioResult, error) {
	conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, err
	}
	defer conn.Close(ctx)
	if s.Setup != "" {
		if _, err := conn.Exec(ctx, s.Setup); err != nil {
			return nil, fmt.Errorf("setup: %w", err)
		}
	}
	if s.Prepare != nil {
		if err := s.Prepare(); err != nil {
			return nil, fmt.Errorf("prepare: %w", err)
		}
	}
	for _, t := range s.Tables {
		q := fmt.Sprintf(`SELECT setval(pg_get_serial_sequence('%[1]s', 'id'), COALESCE((SELECT MAX(id) FROM %[1]s), 0) + 1, false)`, t)
		if _, err := conn.Exec(ctx, q); err != nil {
			return nil, fmt.Errorf("rewind %s: %w", t, err)
		}
	}
	if side.RedisURL != "" {
		opt, err := redis.ParseURL(side.RedisURL)
		if err != nil {
			return nil, err
		}
		rc := redis.NewClient(opt)
		err = rc.FlushDB(ctx).Err()
		rc.Close()
		if err != nil {
			return nil, err
		}
	}
	out := &ScenarioResult{}
	for _, req := range s.Steps {
		r, err := Do(side.BaseURL, req)
		if err != nil {
			return nil, err
		}
		Normalize(r, req.Volatile)
		out.Steps = append(out.Steps, r)
	}
	if s.Settle != nil {
		if err := s.Settle(side); err != nil {
			return nil, fmt.Errorf("settle: %w", err)
		}
	}
	for _, q := range s.Snapshot {
		rows, err := conn.Query(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("snapshot %q: %w", q, err)
		}
		var lines []string
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				rows.Close()
				return nil, err
			}
			parts := make([]string, len(vals))
			for i, v := range vals {
				parts[i] = fmt.Sprintf("%v", v)
			}
			lines = append(lines, strings.Join(parts, " | "))
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		out.Snapshots = append(out.Snapshots, lines)
	}
	for _, u := range s.HTTPSnapshots {
		resp, err := client.Get(u)
		if err != nil {
			return nil, err
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		out.Snapshots = append(out.Snapshots, []string{string(b)})
	}
	return out, nil
}

// CompareScenario lists the differences between two runs of s.
func CompareScenario(s Scenario, a, b *ScenarioResult) []string {
	var out []string
	for i := range s.Steps {
		if d := Compare(a.Steps[i], b.Steps[i]); len(d) > 0 {
			req := s.Steps[i]
			out = append(out, fmt.Sprintf("step %d %s %s %s", i+1, methodOf(req), req.Path, req.Name))
			for _, line := range d {
				out = append(out, "    "+line)
			}
		}
	}
	names := append(append([]string{}, s.Snapshot...), s.HTTPSnapshots...)
	for i, q := range names {
		ra, rb := a.Snapshots[i], b.Snapshots[i]
		if strings.Join(ra, "\n") == strings.Join(rb, "\n") {
			continue
		}
		out = append(out, fmt.Sprintf("snapshot %d (%s): rails %d rows, go %d rows", i+1, firstLine(q), len(ra), len(rb)))
		for j := 0; j < len(ra) || j < len(rb); j++ {
			var x, y string
			if j < len(ra) {
				x = ra[j]
			}
			if j < len(rb) {
				y = rb[j]
			}
			if x != y {
				out = append(out, "    rails: "+x, "    go:    "+y)
				break
			}
		}
	}
	return out
}

func methodOf(r Request) string {
	if r.Method == "" {
		return "GET"
	}
	return r.Method
}

func firstLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}
