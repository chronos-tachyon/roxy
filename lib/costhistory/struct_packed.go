package costhistory

import (
	"time"
)

// Packed represents one data point measuring the cost counter's value over
// time.  The cost counter is a monotonically increasing value which records
// the sum of all costs incurred up to a moment in time.
//
// Unlike Sample, time is recorded as a delta relative to an epoch.
type Packed struct {
	Delta   time.Duration
	Counter uint64
}

// Pack converts Sample to Packed.
func (sample Sample) Pack(epoch time.Time) Packed {
	return Packed{
		Delta:   sample.Time.Sub(epoch),
		Counter: sample.Counter,
	}
}

// Unpack converts Packed to Sample.
func (packed Packed) Unpack(epoch time.Time) Sample {
	return Sample{
		Time:    epoch.Add(packed.Delta),
		Counter: packed.Counter,
	}
}

// PackSamples converts []Sample to []Packed.
func PackSamples(epoch time.Time, in []Sample) []Packed {
	out := make([]Packed, len(in))
	for index, sample := range in {
		out[index] = sample.Pack(epoch)
	}
	return out
}

// UnpackSamples converts []Packed to []Sample.
func UnpackSamples(epoch time.Time, in []Packed) []Sample {
	out := make([]Sample, len(in))
	for index, packed := range in {
		out[index] = packed.Unpack(epoch)
	}
	return out
}
