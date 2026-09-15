package system

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// writeFakeProcess creates a minimal /proc/[pid] directory sufficient for
// FindProcesses, Uptime and MemoryUsage to parse.
func writeFakeProcess(t *testing.T, procDir string, pid, ppid int, comm string, startTicks int, rssKB int) {
	t.Helper()
	dir := filepath.Join(procDir, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "comm"), []byte(comm+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte("nginx\x00-g\x00daemon off;\x00"), 0o644); err != nil {
		t.Fatal(err)
	}

	fields := make([]string, 42)
	for i := range fields {
		fields[i] = "0"
	}
	fields[0] = "S" // state
	fields[1] = strconv.Itoa(ppid)
	fields[19] = strconv.Itoa(startTicks) // starttime, index 19 after state

	statLine := strconv.Itoa(pid) + " (" + comm + ") " + strings.Join(fields, " ")
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(statLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	status := "VmRSS:\t" + strconv.Itoa(rssKB) + " kB\n"
	if err := os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindProcessesIdentifiesMasterAndWorkers(t *testing.T) {
	procDir := t.TempDir()
	writeFakeProcess(t, procDir, 100, 1, "nginx", 0, 10240)  // master, parent is init (pid 1, not nginx)
	writeFakeProcess(t, procDir, 101, 100, "nginx", 0, 5120) // worker
	writeFakeProcess(t, procDir, 102, 100, "nginx", 0, 5120) // worker
	writeFakeProcess(t, procDir, 999, 1, "bash", 0, 1024)    // unrelated process

	info, err := FindProcesses(procDir)
	if err != nil {
		t.Fatalf("FindProcesses: %v", err)
	}
	if info.MasterPID != 100 {
		t.Errorf("expected master pid 100, got %d", info.MasterPID)
	}
	if info.WorkerCount != 2 {
		t.Errorf("expected 2 workers, got %d", info.WorkerCount)
	}
	if info.TotalCount != 3 {
		t.Errorf("expected 3 total nginx processes, got %d", info.TotalCount)
	}
}

func TestMemoryUsage(t *testing.T) {
	procDir := t.TempDir()
	writeFakeProcess(t, procDir, 100, 1, "nginx", 0, 20480)

	mem, err := MemoryUsage(procDir, 100)
	if err != nil {
		t.Fatalf("MemoryUsage: %v", err)
	}
	if mem != 20480*1024 {
		t.Errorf("expected %d bytes, got %d", 20480*1024, mem)
	}
}

func TestUptime(t *testing.T) {
	procDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(procDir, "stat"), []byte("btime 1000000000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// starttime of 100 ticks (1 second at 100 Hz) after boot.
	writeFakeProcess(t, procDir, 100, 1, "nginx", 100, 1024)

	up, err := Uptime(procDir, 100)
	if err != nil {
		t.Fatalf("Uptime: %v", err)
	}
	// Uptime should be roughly (now - (btime+1s)), a large positive value
	// since btime is fixed far in the past relative to "now".
	if up <= 0 {
		t.Errorf("expected positive uptime, got %v", up)
	}
	_ = time.Second
}
