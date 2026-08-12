package telemetry

import (
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Snapshot struct {
	At          time.Time `json:"at"`
	GOOS        string    `json:"goos"`
	GOARCH      string    `json:"goarch"`
	SwapUsage   string    `json:"swap_usage,omitempty"`
	MemoryStats string    `json:"memory_stats,omitempty"`
}

func Capture() Snapshot {
	s := Snapshot{At: time.Now(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	if runtime.GOOS == "darwin" {
		if b, err := exec.Command("sysctl", "-n", "vm.swapusage").CombinedOutput(); err == nil {
			s.SwapUsage = strings.TrimSpace(string(b))
		}
		if b, err := exec.Command("memory_pressure", "-Q").CombinedOutput(); err == nil {
			s.MemoryStats = strings.TrimSpace(string(b))
		}
	} else {
		if b, err := exec.Command("sh", "-c", "free -h 2>/dev/null || true").CombinedOutput(); err == nil {
			s.MemoryStats = strings.TrimSpace(string(b))
		}
	}
	return s
}
