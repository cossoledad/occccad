//go:build linux

package monitoring

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ProcSampler struct {
	mu       sync.Mutex
	previous map[int]cpuSample
}

type cpuSample struct {
	ticks uint64
	at    time.Time
}

func NewProcSampler() *ProcSampler { return &ProcSampler{previous: map[int]cpuSample{}} }

func (sampler *ProcSampler) Sample(process Process) (Process, error) {
	if process.PID <= 0 || !process.Running {
		return process, nil
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", process.PID))
	if err != nil {
		process.Running = false
		return process, err
	}
	end := strings.LastIndexByte(string(stat), ')')
	if end < 0 {
		return process, fmt.Errorf("invalid proc stat for pid %d", process.PID)
	}
	fields := strings.Fields(string(stat)[end+2:])
	if len(fields) < 22 {
		return process, fmt.Errorf("short proc stat for pid %d", process.PID)
	}
	parse := func(index int) uint64 { value, _ := strconv.ParseUint(fields[index], 10, 64); return value }
	ticks := parse(11) + parse(12)
	startTicks, virtualBytes := parse(19), parse(20)
	residentPages, _ := strconv.ParseInt(fields[21], 10, 64)
	threads, _ := strconv.Atoi(fields[17])
	clk := float64(100) // Linux USER_HZ is 100 on supported development targets.
	uptimeRaw, _ := os.ReadFile("/proc/uptime")
	uptimeFields := strings.Fields(string(uptimeRaw))
	hostUptime, _ := strconv.ParseFloat(uptimeFields[0], 64)
	process.ResidentBytes = uint64(max(residentPages, 0)) * uint64(os.Getpagesize())
	process.VirtualBytes, process.Threads = virtualBytes, threads
	process.UptimeSeconds = max(hostUptime-float64(startTicks)/clk, 0)
	now := time.Now()
	sampler.mu.Lock()
	if old, ok := sampler.previous[process.PID]; ok {
		seconds := now.Sub(old.at).Seconds()
		if seconds > 0 {
			process.CPUPercent = float64(ticks-old.ticks) / clk / seconds * 100
		}
	}
	sampler.previous[process.PID] = cpuSample{ticks: ticks, at: now}
	sampler.mu.Unlock()
	return process, nil
}
