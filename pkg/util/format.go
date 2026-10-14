package util

import (
	"fmt"
	"math"
)

// FormatBytes formats bytes into human-readable format (B, KB, MB, GB, TB)
func FormatBytes(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}

	const unit = 1024
	exp := int(math.Log(float64(bytes)) / math.Log(unit))
	if exp == 0 {
		return fmt.Sprintf("%d B", bytes)
	}

	// Define units
	units := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}

	// Calculate the value in the appropriate unit
	value := float64(bytes) / math.Pow(unit, float64(exp))

	// Format with appropriate precision
	if value >= 100 {
		return fmt.Sprintf("%.0f %s", value, units[exp])
	} else if value >= 10 {
		return fmt.Sprintf("%.1f %s", value, units[exp])
	} else {
		return fmt.Sprintf("%.2f %s", value, units[exp])
	}
}

// FormatDuration formats duration into human-readable format
func FormatDuration(seconds float64) string {
	if seconds < 1 {
		return fmt.Sprintf("%.0fms", seconds*1000)
	}
	if seconds < 60 {
		return fmt.Sprintf("%.1fs", seconds)
	}
	if seconds < 3600 {
		minutes := int(seconds / 60)
		remainingSeconds := seconds - float64(minutes*60)
		return fmt.Sprintf("%dm %.1fs", minutes, remainingSeconds)
	}
	hours := int(seconds / 3600)
	remainingMinutes := int((seconds - float64(hours*3600)) / 60)
	return fmt.Sprintf("%dh %dm", hours, remainingMinutes)
}

// FormatPercentage formats a ratio as a percentage
func FormatPercentage(ratio float64) string {
	return fmt.Sprintf("%.1f%%", ratio*100)
}
