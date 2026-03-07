//go:build darwin && cpu_perf

package tests

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// descendantPIDs returns all process IDs that are descendants of rootPID
// using recursive pgrep calls.
func descendantPIDs(rootPID int) []int {
	var result []int
	queue := directChildPIDs(rootPID)
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		result = append(result, pid)
		queue = append(queue, directChildPIDs(pid)...)
	}
	return result
}

func directChildPIDs(parentPID int) []int {
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(parentPID)).Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, s := range strings.Fields(string(out)) {
		pid, err := strconv.Atoi(s)
		if err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}

// chromePIDs returns all descendant PIDs of daemonPID.
func chromePIDs(t *testing.T, daemonPID int) []int {
	t.Helper()
	pids := descendantPIDs(daemonPID)
	if len(pids) == 0 {
		t.Logf("warning: no child processes found under daemon PID %d", daemonPID)
	}
	return pids
}

// sumCPUPercent measures the total CPU% of pids over window d using top -l 2.
//
// top -l 2 -s N collects two samples N seconds apart. The second sample's CPU
// column reflects actual CPU consumption during the interval — a true windowed
// measurement, unlike ps %cpu which is a decaying lifetime average.
func sumCPUPercent(t *testing.T, pids []int, d time.Duration) float64 {
	t.Helper()

	secs := int(d.Seconds())
	if secs < 1 {
		secs = 1
	}

	pidSet := make(map[string]bool, len(pids))
	for _, p := range pids {
		pidSet[strconv.Itoa(p)] = true
	}

	// Collect all processes; -pid is not used because top only accepts one PID
	// filter and we need multiple. The output is small (~few KB) even system-wide.
	out, err := exec.Command("top", "-l", "2", "-s", strconv.Itoa(secs),
		"-stats", "pid,cpu").Output()
	if err != nil {
		t.Logf("top error: %v", err)
		return 0
	}

	// Parse: each PID appears twice (once per sample). The second occurrence
	// is the windowed CPU value. Track which PIDs we've already seen once.
	firstSeen := make(map[string]bool)
	cpuByPID := make(map[string]float64)

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !pidSet[fields[0]] {
			continue
		}
		if firstSeen[fields[0]] {
			v, err := strconv.ParseFloat(fields[1], 64)
			if err == nil {
				cpuByPID[fields[0]] = v
			}
		} else {
			firstSeen[fields[0]] = true
		}
	}

	var total float64
	for _, v := range cpuByPID {
		total += v
	}
	return total
}
