package costhistory

import (
	"time"
)

// Data represents a snapshot of both the absolute cost counter and the average
// rate of growth in CPS.
type Data struct {
	Now       time.Time
	Counter   uint64
	PerSecond float64
}
