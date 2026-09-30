// Package runtimecfg prepares a process for production: it sizes the Go
// runtime to the container and checks the configuration the Rails app would
// have refused to boot without.
package runtimecfg

import (
	"errors"
	"log/slog"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/dodopok/estevao-api-go/internal/config"
)

// Tune sets GOMAXPROCS to the container's CPU quota and the soft memory
// limit to 90% of its memory limit, unless GOMAXPROCS or GOMEMLIMIT is set.
// Go 1.24 reads neither from cgroups: on a shared host it would schedule on
// every host core and be throttled by the quota, and collect garbage
// without regard to the limit the container is killed at.
func Tune(logger *slog.Logger) {
	attrs := []any{}
	if os.Getenv("GOMAXPROCS") == "" {
		if n, ok := cgroupCPUs(); ok && n < runtime.NumCPU() {
			runtime.GOMAXPROCS(n)
		}
	}
	attrs = append(attrs, "gomaxprocs", runtime.GOMAXPROCS(0))
	if os.Getenv("GOMEMLIMIT") == "" {
		if limit, ok := cgroupMemory(); ok {
			debug.SetMemoryLimit(limit / 10 * 9)
			attrs = append(attrs, "gomemlimit_mb", limit/10*9>>20)
		}
	}
	logger.Info("runtime", attrs...)
}

// cgroupRoot is where the cgroup files are mounted (a variable for tests).
var cgroupRoot = "/sys/fs/cgroup"

func readTrim(path string) (string, bool) {
	b, err := os.ReadFile(cgroupRoot + strings.TrimPrefix(path, "/sys/fs/cgroup"))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// cgroupCPUs is ceil(quota / period) from cgroup v2 (cpu.max) or v1.
func cgroupCPUs() (int, bool) {
	var quota, period float64
	if s, ok := readTrim("/sys/fs/cgroup/cpu.max"); ok {
		f := strings.Fields(s)
		if len(f) != 2 || f[0] == "max" {
			return 0, false
		}
		quota, _ = strconv.ParseFloat(f[0], 64)
		period, _ = strconv.ParseFloat(f[1], 64)
	} else {
		q, ok1 := readTrim("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
		p, ok2 := readTrim("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
		if !ok1 || !ok2 {
			return 0, false
		}
		quota, _ = strconv.ParseFloat(q, 64)
		period, _ = strconv.ParseFloat(p, 64)
	}
	if quota <= 0 || period <= 0 {
		return 0, false
	}
	return max(1, int(math.Ceil(quota/period))), true
}

// cgroupMemory is the container's memory limit in bytes.
func cgroupMemory() (int64, bool) {
	s, ok := readTrim("/sys/fs/cgroup/memory.max")
	if !ok {
		s, ok = readTrim("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	}
	if !ok || s == "max" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	// cgroup v1 reports "no limit" as a number near 2^63.
	if err != nil || n <= 0 || n > 1<<50 {
		return 0, false
	}
	return n, true
}

// Check reports configuration errors the service must not start with, and
// logs the ones it may.
func Check(logger *slog.Logger) error {
	if config.RailsEnv() == "production" && strings.TrimSpace(config.Get("SECRET_KEY_BASE")) == "" {
		// Rails refuses to boot without it; here it signs the Active
		// Storage URLs, which an empty secret would make forgeable.
		return errors.New("SECRET_KEY_BASE is required in production")
	}
	if k := config.Get("TRUSTED_SERVER_KEY"); k != "" && k == config.Get("APP_INTERNAL_IDENTIFIER") {
		logger.Error("TRUSTED_SERVER_KEY equals APP_INTERNAL_IDENTIFIER: the identifier ships in the mobile app, so anyone can extract a key that skips rate limiting; set a different secret")
	}
	return nil
}
