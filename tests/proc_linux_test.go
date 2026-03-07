//go:build linux && cpu_perf

package tests

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// descendantPIDs returns all process IDs that are descendants of rootPID by
// building a parent→children map from /proc and doing a BFS from rootPID.
func descendantPIDs(rootPID int) []int {
	children := map[int][]int{}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		ppid := readPPID(pid)
		if ppid > 0 {
			children[ppid] = append(children[ppid], pid)
		}
	}

	var result []int
	queue := children[rootPID]
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		result = append(result, pid)
		queue = append(queue, children[pid]...)
	}
	return result
}

// readPPID reads the parent PID from /proc/[pid]/status.
func readPPID(pid int) int {
	f, err := os.Open(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return -1
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PPid:") {
			v, _ := strconv.Atoi(strings.TrimSpace(line[5:]))
			return v
		}
	}
	return -1
}

// chromePIDs returns all descendant PIDs of daemonPID.
// This captures leakless + Chrome broker + all Chrome child processes
// (renderers, GPU process, utility, network service, etc.).
func chromePIDs(t *testing.T, daemonPID int) []int {
	t.Helper()
	pids := descendantPIDs(daemonPID)
	if len(pids) == 0 {
		t.Logf("warning: no child processes found under daemon PID %d", daemonPID)
	}
	return pids
}

type cpuSnapshot struct {
	ticks uint64
	ts    time.Time
}

// readCPUTicks returns utime+stime for pid from /proc/[pid]/stat.
//
// /proc/[pid]/stat layout (space-separated):
//
//	pid (comm) state ppid ... utime stime ...
//
// The comm field may contain spaces and is wrapped in parens, so we anchor
// parsing at the last ')' in the file.
func readCPUTicks(pid int) (cpuSnapshot, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return cpuSnapshot{}, err
	}
	s := string(data)

	// Skip past "(comm)" — it may contain spaces, so find the last ')'.
	end := strings.LastIndex(s, ")")
	if end < 0 || end+2 >= len(s) {
		return cpuSnapshot{}, fmt.Errorf("malformed /proc/%d/stat", pid)
	}

	// Fields after ')': state(0) ppid(1) pgrp(2) session(3) tty_nr(4)
	//   tpgid(5) flags(6) minflt(7) cminflt(8) majflt(9) cmajflt(10)
	//   utime(11) stime(12) ...
	fields := strings.Fields(s[end+2:])
	if len(fields) < 13 {
		return cpuSnapshot{}, fmt.Errorf("too few fields in /proc/%d/stat", pid)
	}

	utime, _ := strconv.ParseUint(fields[11], 10, 64)
	stime, _ := strconv.ParseUint(fields[12], 10, 64)
	return cpuSnapshot{ticks: utime + stime, ts: time.Now()}, nil
}

// sumCPUPercent measures the total CPU% of all pids over a window of duration d
// using /proc/[pid]/stat delta measurement.
//
//	CPU% = (Δutime + Δstime) / (wall_seconds × CLK_TCK) × 100
//
// CLK_TCK is 100 on virtually all Linux configurations (sysconf(_SC_CLK_TCK)).
// If a process exits during the window it is silently excluded from the sum.
func sumCPUPercent(t *testing.T, pids []int, d time.Duration) float64 {
	t.Helper()
	const clkTck = 100 // sysconf(_SC_CLK_TCK); 100 on virtually all Linux systems

	before := make(map[int]cpuSnapshot, len(pids))
	for _, pid := range pids {
		snap, err := readCPUTicks(pid)
		if err == nil {
			before[pid] = snap
		}
	}

	start := time.Now()
	time.Sleep(d)
	elapsed := time.Since(start).Seconds()

	var deltaTicks uint64
	for _, pid := range pids {
		snap, err := readCPUTicks(pid)
		if err != nil {
			continue // process exited during window; skip it
		}
		pre, ok := before[pid]
		if !ok {
			continue
		}
		if snap.ticks >= pre.ticks {
			deltaTicks += snap.ticks - pre.ticks
		}
	}

	if elapsed <= 0 {
		return 0
	}
	return float64(deltaTicks) / (elapsed * clkTck) * 100
}
