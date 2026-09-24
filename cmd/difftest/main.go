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
	"sort"
	"strings"

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
	flag.Parse()

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
			for _, sc := range scenarios {
				total++
				a, err := diff.RunScenario(ctx, sc, diff.Side{Name: "rails", BaseURL: *railsURL, RedisURL: *railsRedis})
				if err != nil {
					fmt.Fprintf(os.Stderr, "scenario %s (rails): %v\n", sc.Name, err)
					os.Exit(2)
				}
				b, err := diff.RunScenario(ctx, sc, diff.Side{Name: "go", BaseURL: *goURL, RedisURL: *goRedis})
				if err != nil {
					fmt.Fprintf(os.Stderr, "scenario %s (go): %v\n", sc.Name, err)
					os.Exit(2)
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
		for _, req := range reqs {
			total++
			a, err1 := diff.Do(*railsURL, req)
			b, err2 := diff.Do(*goURL, req)
			if err1 != nil || err2 != nil {
				failed++
				sf++
				fmt.Printf("ERROR %s %s: rails=%v go=%v\n", req.Method, req.Path, err1, err2)
				continue
			}
			diff.Normalize(a, req.Volatile)
			diff.Normalize(b, req.Volatile)
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
		fmt.Printf("suite %-24s %5d requests  %5d differences\n", name, len(reqs), sf)
	}
	fmt.Printf("TOTAL %d requests, %d with differences\n", total, failed)
	if failed > 0 {
		os.Exit(1)
	}
}
