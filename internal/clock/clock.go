// Package clock is the one source of the current time for the application.
// It is the real clock unless ESTEVAO_TEST_CLOCK names an instant
// (RFC 3339): then it starts there when the process starts and keeps
// ticking, so the recorded test corpus can be replayed on any later day
// (docs/TESTING.md). Never set it in production.
package clock

import (
	"fmt"
	"os"
	"time"
)

var offset time.Duration

func init() {
	if v := os.Getenv("ESTEVAO_TEST_CLOCK"); v != "" {
		start, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			panic(fmt.Sprintf("ESTEVAO_TEST_CLOCK %q is not an RFC 3339 instant: %v", v, err))
		}
		offset = time.Until(start)
		fmt.Fprintf(os.Stderr, "clock: ESTEVAO_TEST_CLOCK is set, the application believes it is %s (a test clock: never in production)\n", start.Format(time.RFC3339))
	}
}

// Now is time.Now on the application's clock.
func Now() time.Time {
	if offset == 0 {
		return time.Now()
	}
	return time.Now().Add(offset)
}

// Since is time.Since on the application's clock.
func Since(t time.Time) time.Duration { return Now().Sub(t) }

// Shifted reports whether a test clock is in force.
func Shifted() bool { return offset != 0 }

// ChildEnv is what a process started now must inherit to share this
// clock (empty on the real clock): the instant it is now here, since a
// test clock starts at its instant when its own process starts.
func ChildEnv() []string {
	if offset == 0 {
		return nil
	}
	return []string{"ESTEVAO_TEST_CLOCK=" + Now().Format(time.RFC3339Nano)}
}
