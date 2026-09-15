package system

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ListeningPorts returns the TCP ports that any of the given PIDs are
// currently listening on, by cross-referencing each process's open file
// descriptors (socket inodes) against /proc/net/tcp and /proc/net/tcp6.
func ListeningPorts(procDir string, pids []int) ([]int, error) {
	if procDir == "" {
		procDir = "/proc"
	}

	listeningInodes, err := listeningSocketInodes(procDir)
	if err != nil {
		return nil, err
	}

	ports := map[int]bool{}
	for _, pid := range pids {
		fdDir := filepath.Join(procDir, strconv.Itoa(pid), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // process may have exited or be inaccessible; skip it
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if !strings.HasPrefix(link, "socket:[") {
				continue
			}
			inode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
			if port, ok := listeningInodes[inode]; ok {
				ports[port] = true
			}
		}
	}

	result := make([]int, 0, len(ports))
	for p := range ports {
		result = append(result, p)
	}
	return result, nil
}

// listeningSocketInodes parses /proc/net/tcp{,6} and returns a map from
// socket inode (as a string, matching the /proc/[pid]/fd symlink format) to
// local port number, for sockets in the LISTEN state (0A).
func listeningSocketInodes(procDir string) (map[string]int, error) {
	result := map[string]int{}
	for _, name := range []string{"net/tcp", "net/tcp6"} {
		path := filepath.Join(procDir, name)
		f, err := os.Open(path)
		if err != nil {
			continue // tcp6 may not exist on IPv4-only systems
		}
		scanner := bufio.NewScanner(f)
		first := true
		for scanner.Scan() {
			if first {
				first = false
				continue // header line
			}
			fields := strings.Fields(scanner.Text())
			if len(fields) < 10 {
				continue
			}
			state := fields[3]
			if state != "0A" { // TCP_LISTEN
				continue
			}
			localAddr := fields[1]
			inode := fields[9]
			port, err := parsePortFromHexAddr(localAddr)
			if err != nil {
				continue
			}
			result[inode] = port
		}
		f.Close()
	}
	return result, nil
}

func parsePortFromHexAddr(addr string) (int, error) {
	parts := strings.SplitN(addr, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("unexpected address format %q", addr)
	}
	port, err := strconv.ParseInt(parts[1], 16, 32)
	if err != nil {
		return 0, fmt.Errorf("parsing port from %q: %w", addr, err)
	}
	return int(port), nil
}
