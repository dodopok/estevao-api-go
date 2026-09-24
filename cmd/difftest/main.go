// Command difftest replays scenario suites against the Rails oracle and
// the Go server and reports differences.
//
//	go run ./cmd/difftest -suite calendar -rails http://localhost:3000 -go http://localhost:3001
package main

import (
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
	replay := flag.String("replay", "", "only send the suites' requests to this base URL (for effect snapshots)")
	flag.Parse()

	names := suites.Names()
	if *suite != "all" {
		names = strings.Split(*suite, ",")
	}
	sort.Strings(names)
	total, failed := 0, 0
	shown := 0
	for _, name := range names {
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
