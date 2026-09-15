package system

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ProcessInfo describes the running Nginx master/worker processes, read
// directly from /proc so no external dependency is required.
type ProcessInfo struct {
	MasterPID   int
	WorkerPIDs  []int
	WorkerCount int
	TotalCount  int
}

// FindProcesses scans /proc for nginx processes and classifies them into a
// master and its workers by inspecting each process's command line and
// parent PID.
func FindProcesses(procDir string) (ProcessInfo, error) {
	if procDir == "" {
		procDir = "/proc"
	}
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return ProcessInfo{}, fmt.Errorf("reading %s: %w", procDir, err)
	}

	type proc struct {
		pid  int
		ppid int
		cmd  string
	}
	var nginxProcs []proc

	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		comm, _ := os.ReadFile(filepath.Join(procDir, e.Name(), "comm"))
		if strings.TrimSpace(string(comm)) != "nginx" {
			continue
		}
		ppid := readPPID(filepath.Join(procDir, e.Name(), "stat"))
		cmdline, _ := os.ReadFile(filepath.Join(procDir, e.Name(), "cmdline"))
		nginxProcs = append(nginxProcs, proc{pid: pid, ppid: ppid, cmd: string(cmdline)})
	}

	var info ProcessInfo
	pidSet := map[int]bool{}
	for _, p := range nginxProcs {
		pidSet[p.pid] = true
	}
	for _, p := range nginxProcs {
		if !pidSet[p.ppid] {
			// Parent is not another nginx process: this is the master.
			info.MasterPID = p.pid
		}
	}
	for _, p := range nginxProcs {
		if p.pid != info.MasterPID {
			info.WorkerPIDs = append(info.WorkerPIDs, p.pid)
		}
	}
	info.WorkerCount = len(info.WorkerPIDs)
	info.TotalCount = len(nginxProcs)
	return info, nil
}

func readPPID(statPath string) int {
	data, err := os.ReadFile(statPath)
	if err != nil {
		return -1
	}
	// Format: pid (comm) state ppid ...  -- comm may contain spaces/parens,
	// so parse from the last ')' rather than splitting naively.
	s := string(data)
	idx := strings.LastIndexByte(s, ')')
	if idx == -1 || idx+2 >= len(s) {
		return -1
	}
	fields := strings.Fields(s[idx+2:])
	if len(fields) < 2 {
		return -1
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return -1
	}
	return ppid
}

// ResourceUsage is a lightweight snapshot of Nginx's resource consumption,
// summed across master + worker processes.
type ResourceUsage struct {
	CPUPercent   float64 // approximate, based on a single /proc/stat sample
	MemoryBytes  uint64
	ProcessCount int
	Uptime       time.Duration
}

// Uptime returns how long the process at pid has been running, based on
// /proc/[pid]/stat's starttime field and the system boot time.
func Uptime(procDir string, pid int) (time.Duration, error) {
	if procDir == "" {
		procDir = "/proc"
	}
	bootTime, err := bootTime(procDir)
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(filepath.Join(procDir, strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, fmt.Errorf("reading process stat: %w", err)
	}
	s := string(data)
	idx := strings.LastIndexByte(s, ')')
	if idx == -1 {
		return 0, fmt.Errorf("unexpected stat format")
	}
	fields := strings.Fields(s[idx+2:])
	// starttime is field 22 overall, i.e. index 19 after the ppid field we
	// already skipped two of (state, ppid) -- offset 19 from fields[0]=state.
	const starttimeIndex = 19
	if len(fields) <= starttimeIndex {
		return 0, fmt.Errorf("unexpected stat field count")
	}
	clkTck := 100.0 // USER_HZ, standard on Linux
	startTicks, err := strconv.ParseFloat(fields[starttimeIndex], 64)
	if err != nil {
		return 0, fmt.Errorf("parsing starttime: %w", err)
	}
	startSeconds := startTicks / clkTck
	processStart := bootTime.Add(time.Duration(startSeconds * float64(time.Second)))
	return time.Since(processStart), nil
}

func bootTime(procDir string) (time.Time, error) {
	f, err := os.Open(filepath.Join(procDir, "stat"))
	if err != nil {
		return time.Time{}, fmt.Errorf("reading /proc/stat: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "btime ") {
			secs, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "btime")), 10, 64)
			if err != nil {
				return time.Time{}, fmt.Errorf("parsing btime: %w", err)
			}
			return time.Unix(secs, 0), nil
		}
	}
	return time.Time{}, fmt.Errorf("btime not found in /proc/stat")
}

// MemoryUsage returns the resident set size (RSS) in bytes for pid, read
// from /proc/[pid]/status.
func MemoryUsage(procDir string, pid int) (uint64, error) {
	if procDir == "" {
		procDir = "/proc"
	}
	f, err := os.Open(filepath.Join(procDir, strconv.Itoa(pid), "status"))
	if err != nil {
		return 0, fmt.Errorf("reading process status: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			kb, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parsing VmRSS: %w", err)
			}
			return kb * 1024, nil
		}
	}
	return 0, nil
}
