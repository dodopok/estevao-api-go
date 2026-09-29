// Command bench compares the latency and throughput of the Rails app and the
// Go server on the same machine, database and Redis, endpoint by endpoint.
//
// Each endpoint is warmed on both servers, then measured on one server and
// the other in turn (the order alternates per endpoint), with the same
// concurrency and duration. The first response of each server is checked:
// both must answer 200 with the same body, or the endpoint is not compared.
// A cold pass flushes both caches and times distinct uncached requests one
// at a time.
//
//	go run ./cmd/bench -rails http://localhost:3100 -go http://localhost:3001 -c 8 -d 10s
package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

type endpoint struct {
	name string
	path string
	v2   bool
}

var endpoints = []endpoint{
	{"calendar day", "/api/v1/calendar/2026/12/25?preferences[prayer_book_code]=loc_2015", false},
	{"calendar month", "/api/v1/calendar/2026/12?preferences[prayer_book_code]=loc_2015", false},
	{"calendar year", "/api/v1/calendar/2026?preferences[prayer_book_code]=loc_2019", false},
	{"lectionary day", "/api/v1/lectionary/2026/04/05?preferences[prayer_book_code]=loc_2015", false},
	{"daily office (loc_2015 morning)", "/api/v1/daily_office/2026/12/25/morning?preferences[prayer_book_code]=loc_2015", false},
	{"daily office (loc_2019_en evening)", "/api/v1/daily_office/2026/03/18/evening?preferences[prayer_book_code]=loc_2019_en", false},
	{"daily office (awrv_2025_en morning)", "/api/v1/daily_office/2026/06/07/morning?preferences[prayer_book_code]=awrv_2025_en", false},
	{"prayer books", "/api/v1/prayer_books", false},
	{"preferences (loc_2015)", "/api/v1/prayer_books/loc_2015/preferences", false},
	{"v2 day", "/api/v2/days/2026-12-25?book=loc_2015", true},
	{"v2 readings", "/api/v2/days/2026-04-05/readings?book=loc_2015", true},
}

func headers(v2 bool) map[string]string {
	h := map[string]string{"Accept": "application/json", "X-Trusted-Server-Key": os.Getenv("TRUSTED_SERVER_KEY")}
	if v2 {
		h["X-API-Key"] = "estevao_v2fixture00000000000000000000000000000000000000001"
	} else {
		h["X-App-Internal-Id"] = os.Getenv("APP_INTERNAL_IDENTIFIER")
	}
	return h
}

var generatedAt = regexp.MustCompile(`"generated_at":"[^"]*"`)

var client = &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 256}}

func get(base string, e endpoint) (int, [32]byte, time.Duration, error) {
	req, _ := http.NewRequest(http.MethodGet, base+e.path, nil)
	for k, v := range headers(e.v2) {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, [32]byte{}, 0, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	// meta.generated_at (API v2) is the second the response was built.
	body = generatedAt.ReplaceAll(body, []byte(`"generated_at":""`))
	return resp.StatusCode, sha256.Sum256(body), time.Since(start), err
}

type result struct {
	count     int
	errors    int64
	latencies []time.Duration
	elapsed   time.Duration
}

func (r result) pct(p float64) time.Duration {
	if len(r.latencies) == 0 {
		return 0
	}
	return r.latencies[min(len(r.latencies)-1, int(float64(len(r.latencies))*p))]
}

func load(base string, e endpoint, concurrency int, duration time.Duration) result {
	var mu sync.Mutex
	var all []time.Duration
	var errs int64
	deadline := time.Now().Add(duration)
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var local []time.Duration
			for time.Now().Before(deadline) {
				status, _, d, err := get(base, e)
				if err != nil || status != 200 {
					atomic.AddInt64(&errs, 1)
					continue
				}
				local = append(local, d)
			}
			mu.Lock()
			all = append(all, local...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	return result{count: len(all), errors: errs, latencies: all, elapsed: time.Since(start)}
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1f", float64(d.Microseconds())/1000) }

func flush(url string) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return
	}
	c := redis.NewClient(opt)
	defer c.Close()
	c.FlushDB(context.Background())
}

func main() {
	railsURL := flag.String("rails", "http://localhost:3100", "Rails base URL")
	goURL := flag.String("go", "http://localhost:3001", "Go base URL")
	concurrency := flag.Int("c", 8, "concurrent clients")
	duration := flag.Duration("d", 10*time.Second, "measurement per endpoint and server")
	railsRedis := flag.String("rails-redis", "redis://localhost:6379/1", "Rails cache (flushed for the cold pass)")
	goRedis := flag.String("go-redis", "redis://localhost:6379/2", "Go cache (flushed for the cold pass)")
	only := flag.String("only", "", "measure only endpoints whose name contains this text (skips the cold pass)")
	flag.Parse()
	servers := []struct{ name, url string }{{"rails", *railsURL}, {"go", *goURL}}

	fmt.Printf("## Warm (cached) throughput: %d clients, %s per endpoint and server\n\n", *concurrency, *duration)
	fmt.Println("| Endpoint | Server | req/s | p50 ms | p95 ms | p99 ms | errors |")
	fmt.Println("|---|---|---:|---:|---:|---:|---:|")
	for i, e := range endpoints {
		if *only != "" && !strings.Contains(e.name, *only) {
			continue
		}
		var hashes [][32]byte
		ok := true
		for _, s := range servers {
			for w := 0; w < 20; w++ {
				status, h, _, err := get(s.url, e)
				if err != nil || status != 200 {
					fmt.Fprintf(os.Stderr, "%s %s: status %d %v\n", s.name, e.path, status, err)
					ok = false
					break
				}
				if w == 0 {
					hashes = append(hashes, h)
				}
			}
		}
		if !ok || len(hashes) != 2 || hashes[0] != hashes[1] {
			fmt.Printf("| %s | (skipped: responses differ or fail) | | | | | |\n", e.name)
			continue
		}
		order := servers
		if i%2 == 1 {
			order = []struct{ name, url string }{servers[1], servers[0]}
		}
		rows := map[string]result{}
		for _, s := range order {
			rows[s.name] = load(s.url, e, *concurrency, *duration)
		}
		for _, s := range servers {
			r := rows[s.name]
			fmt.Printf("| %s | %s | %.0f | %s | %s | %s | %d |\n", e.name, s.name, float64(r.count)/r.elapsed.Seconds(),
				ms(r.pct(0.50)), ms(r.pct(0.95)), ms(r.pct(0.99)), r.errors)
		}
	}

	if *only != "" {
		return
	}
	fmt.Printf("\n## Cold (uncached) latency: caches flushed, 20 distinct dates per endpoint, one request at a time\n\n")
	fmt.Println("| Endpoint | Server | mean ms | p50 ms | max ms |")
	fmt.Println("|---|---|---:|---:|---:|")
	cold := []struct {
		name, pattern string
		v2            bool
	}{
		{"daily office (loc_2015 morning)", "/api/v1/daily_office/2027/%02d/%02d/morning?preferences[prayer_book_code]=loc_2015", false},
		{"calendar day (loc_2015)", "/api/v1/calendar/2027/%02d/%02d?preferences[prayer_book_code]=loc_2015", false},
		{"v2 day (loc_2015)", "/api/v2/days/2027-%02d-%02d?book=loc_2015", true},
	}
	for _, c := range cold {
		for _, s := range servers {
			flush(*railsRedis)
			flush(*goRedis)
			var ds []time.Duration
			for i := 0; i < 20; i++ {
				e := endpoint{path: fmt.Sprintf(c.pattern, 1+i%12, 1+i), v2: c.v2}
				status, _, d, err := get(s.url, e)
				if err == nil && status == 200 {
					ds = append(ds, d)
				}
			}
			sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
			var sum time.Duration
			for _, d := range ds {
				sum += d
			}
			if len(ds) == 0 {
				fmt.Printf("| %s | %s | failed | | |\n", c.name, s.name)
				continue
			}
			fmt.Printf("| %s | %s | %s | %s | %s |\n", c.name, s.name, ms(sum/time.Duration(len(ds))), ms(ds[len(ds)/2]), ms(ds[len(ds)-1]))
		}
	}
}
