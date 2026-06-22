// Package system collects host metrics via gopsutil: one-shot snapshots for the
// summary endpoint and an incremental Sampler for the live stream (rates need
// two readings, so the Sampler keeps the previous counters).
package system

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

// Summary is a one-shot snapshot of system state.
type Summary struct {
	Time     int64      `json:"time"`
	Host     HostInfo   `json:"host"`
	CPU      CPUInfo    `json:"cpu"`
	Memory   MemInfo    `json:"memory"`
	Swap     SwapInfo   `json:"swap"`
	Load     *LoadInfo  `json:"load,omitempty"`
	Disks    []DiskInfo `json:"disks"`
	Networks []NetInfo  `json:"networks"`
}

type HostInfo struct {
	Hostname        string `json:"hostname"`
	OS              string `json:"os"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platformVersion"`
	KernelVersion   string `json:"kernelVersion"`
	KernelArch      string `json:"kernelArch"`
	Uptime          uint64 `json:"uptime"`
	BootTime        uint64 `json:"bootTime"`
	Procs           uint64 `json:"procs"`
}

type CPUInfo struct {
	Cores         int       `json:"cores"`
	PhysicalCores int       `json:"physicalCores"`
	ModelName     string    `json:"modelName"`
	Mhz           float64   `json:"mhz"`
	Percent       float64   `json:"percent"`
	PerCore       []float64 `json:"perCore"`
}

type MemInfo struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Available   uint64  `json:"available"`
	UsedPercent float64 `json:"usedPercent"`
}

type SwapInfo struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	UsedPercent float64 `json:"usedPercent"`
}

type LoadInfo struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

type DiskInfo struct {
	Device      string  `json:"device"`
	Mountpoint  string  `json:"mountpoint"`
	Fstype      string  `json:"fstype"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"usedPercent"`
}

type NetInfo struct {
	Name      string `json:"name"`
	BytesSent uint64 `json:"bytesSent"`
	BytesRecv uint64 `json:"bytesRecv"`
}

// Collect gathers a full snapshot. Best-effort: subsystems that error (e.g.
// load average on non-Linux) are simply omitted rather than failing the call.
func Collect() (*Summary, error) {
	s := &Summary{Time: time.Now().Unix()}

	if hi, err := host.Info(); err == nil {
		s.Host = HostInfo{
			Hostname:        hi.Hostname,
			OS:              hi.OS,
			Platform:        hi.Platform,
			PlatformVersion: hi.PlatformVersion,
			KernelVersion:   hi.KernelVersion,
			KernelArch:      hi.KernelArch,
			Uptime:          hi.Uptime,
			BootTime:        hi.BootTime,
			Procs:           hi.Procs,
		}
	}

	s.CPU.Cores, _ = cpu.Counts(true)
	s.CPU.PhysicalCores, _ = cpu.Counts(false)
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		s.CPU.ModelName = infos[0].ModelName
		s.CPU.Mhz = infos[0].Mhz
	}
	if mhz := currentCPUMHz(); mhz > 0 {
		s.CPU.Mhz = mhz
	}
	if per, err := cpu.Percent(200*time.Millisecond, true); err == nil {
		s.CPU.PerCore = per
		s.CPU.Percent = average(per)
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		s.Memory = MemInfo{Total: vm.Total, Used: vm.Used, Available: vm.Available, UsedPercent: vm.UsedPercent}
	}
	if sw, err := mem.SwapMemory(); err == nil {
		s.Swap = SwapInfo{Total: sw.Total, Used: sw.Used, UsedPercent: sw.UsedPercent}
	}
	if la, err := load.Avg(); err == nil {
		s.Load = &LoadInfo{Load1: la.Load1, Load5: la.Load5, Load15: la.Load15}
	}

	if parts, err := disk.Partitions(false); err == nil {
		for _, p := range parts {
			usage, err := disk.Usage(p.Mountpoint)
			if err != nil || usage.Total == 0 {
				continue
			}
			s.Disks = append(s.Disks, DiskInfo{
				Device:      p.Device,
				Mountpoint:  p.Mountpoint,
				Fstype:      p.Fstype,
				Total:       usage.Total,
				Used:        usage.Used,
				Free:        usage.Free,
				UsedPercent: usage.UsedPercent,
			})
		}
	}

	if io, err := net.IOCounters(true); err == nil {
		for _, c := range io {
			if c.Name == "lo" || (c.BytesRecv == 0 && c.BytesSent == 0) {
				continue
			}
			s.Networks = append(s.Networks, NetInfo{Name: c.Name, BytesSent: c.BytesSent, BytesRecv: c.BytesRecv})
		}
	}

	return s, nil
}

// Sample is a lightweight periodic reading for the live stream.
type Sample struct {
	Time                 int64     `json:"time"`
	CPUPercent           float64   `json:"cpuPercent"`
	PerCore              []float64 `json:"perCore"`
	MemUsed              uint64    `json:"memUsed"`
	MemTotal             uint64    `json:"memTotal"`
	MemUsedPercent       float64   `json:"memUsedPercent"`
	SwapUsed             uint64    `json:"swapUsed"`
	SwapTotal            uint64    `json:"swapTotal"`
	SwapUsedPercent      float64   `json:"swapUsedPercent"`
	Load1                float64   `json:"load1"`
	CPUMhz               float64   `json:"cpuMhz"`
	NetRxBytesPerSec     float64   `json:"netRxBytesPerSec"`
	NetTxBytesPerSec     float64   `json:"netTxBytesPerSec"`
	DiskReadBytesPerSec  float64   `json:"diskReadBytesPerSec"`
	DiskWriteBytesPerSec float64   `json:"diskWriteBytesPerSec"`
}

// Sampler produces successive Samples. CPU% is computed from the delta in
// per-core CPU times since the previous Sample, kept on the Sampler itself —
// it does NOT use gopsutil's shared global cpu.Percent(0,...) state, so multiple
// concurrent streams (tabs, reconnects) never interfere. Network rates are
// bytes/sec since the previous Sample.
type Sampler struct {
	lastCPU       []cpu.TimesStat
	lastRx        uint64
	lastTx        uint64
	lastDiskRead  uint64
	lastDiskWrite uint64
	lastTime      time.Time
	primed        bool
}

// NewSampler primes the CPU, network and disk counters.
func NewSampler() *Sampler {
	s := &Sampler{}
	if t, err := cpu.Times(true); err == nil {
		s.lastCPU = t
	}
	if io, err := net.IOCounters(false); err == nil && len(io) > 0 {
		s.lastRx, s.lastTx = io[0].BytesRecv, io[0].BytesSent
	}
	s.lastDiskRead, s.lastDiskWrite = diskIOTotals()
	s.lastTime = time.Now()
	s.primed = true
	return s
}

// Sample reads current metrics, computing CPU% and network rates against the
// previous reading.
func (s *Sampler) Sample() (*Sample, error) {
	now := time.Now()
	out := &Sample{Time: now.Unix()}

	if cur, err := cpu.Times(true); err == nil {
		prev := make(map[string]cpu.TimesStat, len(s.lastCPU))
		for _, p := range s.lastCPU {
			prev[p.CPU] = p
		}
		per := make([]float64, 0, len(cur))
		var sum float64
		var n int
		for _, c := range cur {
			if p, ok := prev[c.CPU]; ok {
				b := busyPercent(p, c)
				per = append(per, b)
				sum += b
				n++
			} else {
				per = append(per, 0)
			}
		}
		out.PerCore = per
		if n > 0 {
			out.CPUPercent = sum / float64(n)
		}
		s.lastCPU = cur
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		out.MemUsed, out.MemTotal, out.MemUsedPercent = vm.Used, vm.Total, vm.UsedPercent
	}
	if sw, err := mem.SwapMemory(); err == nil {
		out.SwapUsed = sw.Used
		out.SwapTotal = sw.Total
		out.SwapUsedPercent = sw.UsedPercent
	}
	if la, err := load.Avg(); err == nil {
		out.Load1 = la.Load1
	}
	out.CPUMhz = currentCPUMHz()

	elapsed := now.Sub(s.lastTime).Seconds()
	if io, err := net.IOCounters(false); err == nil && len(io) > 0 {
		rx, tx := io[0].BytesRecv, io[0].BytesSent
		if s.primed && elapsed > 0 {
			out.NetRxBytesPerSec = float64(deltaU64(rx, s.lastRx)) / elapsed
			out.NetTxBytesPerSec = float64(deltaU64(tx, s.lastTx)) / elapsed
		}
		s.lastRx, s.lastTx = rx, tx
	}
	dr, dw := diskIOTotals()
	if s.primed && elapsed > 0 {
		out.DiskReadBytesPerSec = float64(deltaU64(dr, s.lastDiskRead)) / elapsed
		out.DiskWriteBytesPerSec = float64(deltaU64(dw, s.lastDiskWrite)) / elapsed
	}
	s.lastDiskRead, s.lastDiskWrite = dr, dw
	s.lastTime, s.primed = now, true
	return out, nil
}

// diskIOTotals sums read/write bytes across real block devices (skipping loop
// and ram devices), for computing aggregate disk throughput.
func diskIOTotals() (read, write uint64) {
	io, err := disk.IOCounters()
	if err != nil {
		return 0, 0
	}
	for name, c := range io {
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}
		read += c.ReadBytes
		write += c.WriteBytes
	}
	return read, write
}

// currentCPUMHz returns the average current CPU frequency in MHz. It prefers the
// live cpufreq scaling value from sysfs (accurate on ARM/Raspberry Pi, where
// /proc/cpuinfo has no "cpu MHz"), falling back to gopsutil's static value.
func currentCPUMHz() float64 {
	if paths, _ := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/cpufreq/scaling_cur_freq"); len(paths) > 0 {
		var sum float64
		var n int
		for _, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			if khz, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil && khz > 0 {
				sum += khz / 1000.0
				n++
			}
		}
		if n > 0 {
			return sum / float64(n)
		}
	}
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 && infos[0].Mhz > 0 {
		return infos[0].Mhz
	}
	return 0
}

// busyPercent returns the busy CPU percentage between two cumulative readings.
func busyPercent(prev, cur cpu.TimesStat) float64 {
	totalDelta := cpuTotal(cur) - cpuTotal(prev)
	if totalDelta <= 0 {
		return 0
	}
	idleDelta := (cur.Idle + cur.Iowait) - (prev.Idle + prev.Iowait)
	busy := (totalDelta - idleDelta) / totalDelta * 100
	if busy < 0 {
		return 0
	}
	if busy > 100 {
		return 100
	}
	return busy
}

// cpuTotal sums CPU time fields. Guest/GuestNice are excluded because Linux
// already accounts them within User/Nice.
func cpuTotal(t cpu.TimesStat) float64 {
	return t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
}

func average(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// deltaU64 guards against counter resets (e.g. interface restart).
func deltaU64(cur, prev uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}
