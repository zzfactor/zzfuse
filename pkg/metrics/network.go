package metrics

import (
	"time"

	"github.com/shirou/gopsutil/v3/net"
)

// NetSample captures byte counters at a moment in time.
type NetSample struct {
	InBytes  uint64
	OutBytes uint64
	ErrIn    uint64
	DropIn   uint64
	when     time.Time
}

// SampleNetwork returns aggregated network I/O counters across interfaces.
func SampleNetwork() NetSample {
	io, err := net.IOCounters(false)
	s := NetSample{when: time.Now()}
	if err != nil || len(io) == 0 {
		return s
	}
	s.InBytes = io[0].BytesRecv
	s.OutBytes = io[0].BytesSent
	s.ErrIn = io[0].Errin
	s.DropIn = io[0].Dropin
	return s
}

// Rate calculates per-second rates between two samples.
func (curr NetSample) Rate(prev NetSample) map[string]float64 {
	dt := curr.when.Sub(prev.when).Seconds()
	if dt <= 0 {
		dt = 1
	}
	return map[string]float64{
		"in_bytes_per_s":  float64(curr.InBytes-prev.InBytes) / dt,
		"out_bytes_per_s": float64(curr.OutBytes-prev.OutBytes) / dt,
		"errors_per_s":    float64(curr.ErrIn-prev.ErrIn) / dt,
		"dropped_per_s":   float64(curr.DropIn-prev.DropIn) / dt,
	}
}


