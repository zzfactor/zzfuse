package repository

import (
	"time"
)

// TimeRange defines a start and end time for queries.
type TimeRange struct {
	Start *time.Time
	End   *time.Time
}
