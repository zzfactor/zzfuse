package auth

import (
	"strconv"
	"sync"
	"time"
)

type rateLimitEntry struct {
	count     int
	timestamp time.Time
}

var (
	rateLimitMu sync.Mutex
	rateLimits  = make(map[string]rateLimitEntry) // key: action:userID
)

// AllowAction returns true if the user is allowed to perform the action (max 10/min per user/action).
func AllowAction(userID int64, action string) bool {
	key := action + ":" + strconv.FormatInt(userID, 10)
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()
	entry, ok := rateLimits[key]
	now := time.Now().UTC()
	if !ok || now.Sub(entry.timestamp) > time.Minute {
		rateLimits[key] = rateLimitEntry{count: 1, timestamp: now}
		return true
	}
	if entry.count >= 10 {
		return false
	}
	entry.count++
	rateLimits[key] = entry
	return true
}
