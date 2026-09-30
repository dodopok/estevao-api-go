// Command difftest replays scenario suites against the Rails oracle and
// the Go server and reports differences.
//
//	go run ./cmd/difftest -suite calendar -rails http://localhost:3000 -go http://localhost:3001
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dodopok/estevao-api-go/internal/clock"
	"github.com/dodopok/estevao-api-go/test/diff"
	"github.com/dodopok/estevao-api-go/test/diff/suites"
)

func main() {
	railsURL := flag.String("rails", "http://localhost:3000", "Rails oracle base URL")
	goURL := flag.String("go", "http://localhost:3001", "Go server base URL")
	suite := flag.String("suite", "all", "suite name (comma separated) or all")
	maxFail := flag.Int("show", 30, "failures to print")
	railsRedis := flag.String("rails-redis", "redis://localhost:6379/1", "the oracle's cache (flushed before each scenario)")
	goRedis := flag.String("go-redis", "redis://localhost:6379/2", "the Go server's cache (flushed before each scenario)")
	replay := flag.String("replay", "", "only send the suites' requests to this base URL (for effect snapshots)")
	dump := flag.String("dump", "", "write each scenario's steps and snapshots, per side, under this directory")
	record := flag.String("record", "", "also write the oracle's normalized answers to this corpus directory")
	recordedAt := flag.String("recorded-at", "", "record: the instant of the database snapshot (RFC 3339; default now)")
	corpus := flag.String("corpus", "", "compare the Go server with this recorded corpus instead of a running oracle")
	flag.Parse()
	if *record != "" && *corpus != "" {
		fmt.Fprintln(os.Stderr, "-record and -corpus are exclusive")
		os.Exit(2)
	}
	meta := diff.Meta{RecordedAt: *recordedAt, Suites: map[string]int{}}
	if *record != "" && meta.RecordedAt == "" {
		meta.RecordedAt = clock.Now().UTC().Format(time.RFC3339Nano)
	}
	if *corpus != "" {
		m, err := diff.ReadMeta(*corpus)
		if err != nil {
			fmt.Fprintln(os.Stderr, "corpus:", err)
			os.Exit(2)
		}
		if !clock.Shifted() {
			fmt.Fprintf(os.Stderr, "corpus recorded at %s: run the harness and the Go server with ESTEVAO_TEST_CLOCK=%s (test/corpus/replay.sh does)\n", m.RecordedAt, m.RecordedAt)
			os.Exit(2)
		}
		meta = m
	}

	ctx := context.Background()
	names := suites.Names()
	if *suite != "all" {
		names = strings.Split(*suite, ",")
	}
	sort.Strings(names)
	total, failed := 0, 0
	shown := 0
	for _, name := range names {
		if scenarios, ok := suites.Scenarios(name); ok {
			if *replay != "" {
				continue
			}
			sf := 0
			var recorded []diff.Recorded
			var writer *diff.CorpusWriter
			if *corpus != "" {
				recorded = mustReadCorpus(*corpus, name, len(scenarios))
			}
			if *record != "" {
				writer = mustCorpusWriter(*record, name)
			}
			for i, sc := range scenarios {
				total++
				var a *diff.ScenarioResult
				if *corpus != "" {
					if recorded[i].Kind != "scenario" || recorded[i].Name != sc.Name {
						fmt.Printf("STALE [%s] scenario %d is %q, the corpus has %q: record again\n", name, i+1, sc.Name, recorded[i].Name)
						failed++
						sf++
						continue
					}
					a = recorded[i].Scenario
				} else {
					var err error
					a, err = diff.RunScenario(ctx, sc, diff.Side{Name: "rails", BaseURL: *railsURL, RedisURL: *railsRedis})
					if err != nil {
						fmt.Fprintf(os.Stderr, "scenario %s (rails): %v\n", sc.Name, err)
						os.Exit(2)
					}
				}
				if writer != nil {
					mustWrite(writer, diff.Recorded{Kind: "scenario", Index: i, Name: sc.Name, Scenario: a})
				}
				b, err := diff.RunScenario(ctx, sc, diff.Side{Name: "go", BaseURL: *goURL, RedisURL: *goRedis})
				if err != nil {
					fmt.Fprintf(os.Stderr, "scenario %s (go): %v\n", sc.Name, err)
					os.Exit(2)
				}
				if *dump != "" {
					dumpScenario(*dump, name, sc.Name, "rails", a)
					dumpScenario(*dump, name, sc.Name, "go", b)
				}
				if d := diff.CompareScenario(sc, a, b); len(d) > 0 {
					failed++
					sf++
					if shown < *maxFail {
						shown++
						fmt.Printf("DIFF [%s] scenario %s\n", name, sc.Name)
						for _, line := range d {
							fmt.Println("   ", line)
						}
					}
				}
			}
			if writer != nil {
				meta.Suites[name] = mustClose(writer)
			}
			fmt.Printf("suite %-24s %5d scenarios %5d differences\n", name, len(scenarios), sf)
			continue
		}
		reqs, err := suites.Build(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		sf := 0
		if *replay != "" {
			for _, req := range reqs {
				if _, err := diff.Do(*replay, req); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
			}
			fmt.Printf("suite %-24s %5d requests replayed\n", name, len(reqs))
			continue
		}
		var recorded []diff.Recorded
		var writer *diff.CorpusWriter
		if *corpus != "" {
			recorded = mustReadCorpus(*corpus, name, len(reqs))
		}
		if *record != "" {
			writer = mustCorpusWriter(*record, name)
		}
		for i, req := range reqs {
			total++
			var a *diff.Result
			var err1 error
			if *corpus != "" {
				if !recorded[i].SameRequest(req) {
					fmt.Printf("STALE [%s] request %d is %s %s, the corpus has %s %s: record again\n", name, i+1, req.Method, req.Path, recorded[i].Method, recorded[i].Path)
					failed++
					sf++
					continue
				}
				a = recorded[i].Result
			} else {
				a, err1 = diff.Do(*railsURL, req)
			}
			b, err2 := diff.Do(*goURL, req)
			if err1 != nil || err2 != nil {
				failed++
				sf++
				fmt.Printf("ERROR %s %s: rails=%v go=%v\n", req.Method, req.Path, err1, err2)
				continue
			}
			if *corpus == "" {
				diff.NormalizeRequest(a, req)
			}
			diff.NormalizeRequest(b, req)
			if writer != nil {
				mustWrite(writer, diff.Recorded{Kind: "request", Index: i, Method: methodOf(req), Path: req.Path, Name: req.Name, Result: a})
			}
			if d := diff.Compare(a, b); len(d) > 0 {
				failed++
				sf++
				if shown < *maxFail {
					shown++
					fmt.Printf("DIFF [%s] %s %s %s\n", name, req.Method, req.Path, req.Name)
					for _, line := range d {
						fmt.Println("   ", line)
					}
				}
			}
		}
		if writer != nil {
			meta.Suites[name] = mustClose(writer)
		}
		fmt.Printf("suite %-24s %5d requests  %5d differences\n", name, len(reqs), sf)
	}
	if *record != "" {
		if err := diff.WriteMeta(*record, meta); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	}
	fmt.Printf("TOTAL %d requests, %d with differences\n", total, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// dumpScenario writes one side's run as text for inspection.
func dumpScenario(dir, suite, scenario, side string, r *diff.ScenarioResult) {
	var b strings.Builder
	for i, st := range r.Steps {
		fmt.Fprintf(&b, "== step %d: %d\n%s\n", i+1, st.Status, st.Body)
	}
	for i, snap := range r.Snapshots {
		fmt.Fprintf(&b, "== snapshot %d (%d rows)\n%s\n", i+1, len(snap), strings.Join(snap, "\n"))
	}
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, suite+"."+scenario+"."+side+".txt"), []byte(b.String()), 0o644)
}

func methodOf(r diff.Request) string {
	if r.Method == "" {
		return "GET"
	}
	return r.Method
}

func mustReadCorpus(dir, suite string, want int) []diff.Recorded {
	rec, err := diff.ReadCorpus(dir, suite)
	if err != nil {
		fmt.Fprintf(os.Stderr, "corpus %s: %v\n", suite, err)
		os.Exit(2)
	}
	for len(rec) < want {
		rec = append(rec, diff.Recorded{Kind: "missing"})
	}
	return rec
}

func mustCorpusWriter(dir, suite string) *diff.CorpusWriter {
	w, err := diff.NewCorpusWriter(dir, suite)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	return w
}

func mustWrite(w *diff.CorpusWriter, r diff.Recorded) {
	if err := w.Write(r); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

func mustClose(w *diff.CorpusWriter) int {
	n, err := w.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	return n
}
