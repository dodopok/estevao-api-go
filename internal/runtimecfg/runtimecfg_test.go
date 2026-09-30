package runtimecfg

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func withFiles(t *testing.T, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, name)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := cgroupRoot
	cgroupRoot = dir
	t.Cleanup(func() { cgroupRoot = old })
}

func TestCgroupV2(t *testing.T) {
	withFiles(t, map[string]string{"cpu.max": "150000 100000", "memory.max": "536870912"})
	if n, ok := cgroupCPUs(); !ok || n != 2 {
		t.Fatalf("cpus %d %v", n, ok)
	}
	if m, ok := cgroupMemory(); !ok || m != 512<<20 {
		t.Fatalf("memory %d %v", m, ok)
	}
}

func TestCgroupV2Unlimited(t *testing.T) {
	withFiles(t, map[string]string{"cpu.max": "max 100000", "memory.max": "max"})
	if _, ok := cgroupCPUs(); ok {
		t.Fatal("unlimited CPU read as a quota")
	}
	if _, ok := cgroupMemory(); ok {
		t.Fatal("unlimited memory read as a limit")
	}
}

func TestCgroupV1(t *testing.T) {
	withFiles(t, map[string]string{
		"cpu/cpu.cfs_quota_us": "50000", "cpu/cpu.cfs_period_us": "100000",
		"memory/memory.limit_in_bytes": "9223372036854771712",
	})
	if n, ok := cgroupCPUs(); !ok || n != 1 {
		t.Fatalf("cpus %d %v", n, ok)
	}
	if _, ok := cgroupMemory(); ok {
		t.Fatal("v1's no-limit value read as a limit")
	}
}

func TestCheck(t *testing.T) {
	t.Setenv("RAILS_ENV", "production")
	t.Setenv("SECRET_KEY_BASE", "")
	if Check(nilLogger()) == nil {
		t.Fatal("production without SECRET_KEY_BASE accepted")
	}
	t.Setenv("SECRET_KEY_BASE", "x")
	if err := Check(nilLogger()); err != nil {
		t.Fatal(err)
	}
}

func nilLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
