package spgw

import "time"

// nowSub returns seconds since t, rounded down.
func nowSub(t time.Time) int64 {
	return int64(time.Since(t).Seconds())
}
