package metrics

import (
	"os"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

// GetSystemMetrics returns CPU and memory utilization for the host and process.
// Keys are intentionally lowercase to align with existing perfmonitor payload style.
func GetSystemMetrics() map[string]interface{} {
	result := make(map[string]interface{})

	// CPU utilization (system-wide)
	// Using a short sample to provide a point-in-time percentage without blocking too long.
	if pct, err := cpu.Percent(100*time.Millisecond, false); err == nil && len(pct) > 0 {
		result["cpu_system_percent"] = pct[0]
	} else {
		result["cpu_system_percent"] = 0.0
	}

	// CPU utilization (this process)
	if p, err := process.NewProcess(int32(os.Getpid())); err == nil {
		if ppct, err := p.Percent(0); err == nil {
			result["cpu_process_percent"] = ppct
		} else {
			result["cpu_process_percent"] = 0.0
		}
	} else {
		result["cpu_process_percent"] = 0.0
	}

	// Memory utilization (system)
	if vm, err := mem.VirtualMemory(); err == nil {
		result["mem_util_percent"] = vm.UsedPercent
	} else {
		result["mem_util_percent"] = 0.0
	}

	return result
}


