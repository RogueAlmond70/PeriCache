package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ensureListenAddr makes addr available for binding. When replace is true and the
// port is already taken, the process holding it is terminated so a new server can start.
// It returns how many processes were stopped.
func ensureListenAddr(addr string, replace bool) (int, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		_ = ln.Close()
		return 0, nil
	}
	if !replace || !isAddrInUse(err) {
		return 0, err
	}

	port, err := portFromAddr(addr)
	if err != nil {
		return 0, err
	}
	pids, err := pidsListeningOnPort(port)
	if err != nil {
		return 0, fmt.Errorf("address %s in use, and failed to find process: %w", addr, err)
	}
	self := os.Getpid()
	killed := 0
	for _, pid := range pids {
		if pid <= 0 || pid == self {
			continue
		}
		if err := killProcess(pid); err != nil {
			return killed, fmt.Errorf("address %s in use by pid %d: %w", addr, pid, err)
		}
		killed++
	}
	if killed == 0 {
		return 0, fmt.Errorf("listen tcp %s: address already in use", addr)
	}

	// Brief pause so the OS releases the socket after taskkill.
	time.Sleep(200 * time.Millisecond)

	ln, err = net.Listen("tcp", addr)
	if err != nil {
		return killed, err
	}
	_ = ln.Close()
	return killed, nil
}

func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	if op, ok := err.(*net.OpError); ok {
		err = op.Err
	}
	if sys, ok := err.(*os.SyscallError); ok {
		err = sys.Err
	}
	if errno, ok := err.(syscall.Errno); ok {
		// Windows WSAEADDRINUSE is 10048; Unix EADDRINUSE varies.
		return errno == syscall.EADDRINUSE || uintptr(errno) == 10048
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "only one usage of each socket address")
}

func portFromAddr(addr string) (string, error) {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		// Allow ":8080" style — SplitHostPort handles it.
		return "", fmt.Errorf("invalid listen addr %q: %w", addr, err)
	}
	if port == "" {
		return "", fmt.Errorf("invalid listen addr %q: missing port", addr)
	}
	return port, nil
}

func pidsListeningOnPort(port string) ([]int, error) {
	if runtime.GOOS == "windows" {
		return pidsListeningOnPortWindows(port)
	}
	return pidsListeningOnPortUnix(port)
}

func pidsListeningOnPortWindows(port string) ([]int, error) {
	out, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
	if err != nil {
		return nil, err
	}
	suffix := ":" + port
	seen := map[int]struct{}{}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if !strings.EqualFold(fields[0], "TCP") {
			continue
		}
		if !strings.HasSuffix(fields[1], suffix) {
			continue
		}
		state := strings.ToUpper(fields[3])
		if state != "LISTENING" && state != "LISTEN" {
			continue
		}
		pid, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil || pid <= 0 {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		pids = append(pids, pid)
	}
	return pids, nil
}

func pidsListeningOnPortUnix(port string) ([]int, error) {
	// lsof is widely available on macOS/Linux for this use case.
	out, err := exec.Command("lsof", "-nP", "-iTCP:"+port, "-sTCP:LISTEN", "-t").Output()
	if err != nil {
		// Exit code 1 from lsof usually means no matches.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}
	seen := map[int]struct{}{}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil || pid <= 0 {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		pids = append(pids, pid)
	}
	return pids, nil
}

func killProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		// Signal handling differs on Windows; Terminate is the reliable local-dev approach.
		return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/F").Run()
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return proc.Kill()
	}
	return nil
}
