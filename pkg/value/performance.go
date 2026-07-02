package value

import "time"

// perfOrigin is the instant this process started measuring time, the time origin
// performance.now() counts from. It is captured once at package init so every
// PerformanceNow reading shares one origin, which is what makes two readings
// subtract to a real elapsed duration. The value carries a monotonic clock
// reading, so time.Since below is immune to a wall-clock adjustment during the
// run.
var perfOrigin = time.Now()

// PerformanceNow returns the number of milliseconds since the time origin, the
// lowering of performance.now(). It is a float64 with sub-millisecond resolution,
// matching the DOMHighResTimeStamp performance.now() returns in a browser and in
// Node, where the value is a fractional count of milliseconds rather than a whole
// number. Only differences between two readings are meaningful, so the absolute
// origin is an implementation detail; what matters is that every reading measures
// from the same monotonic start.
func PerformanceNow() float64 {
	return float64(time.Since(perfOrigin).Nanoseconds()) / 1e6
}
