package costhistory

import (
	"time"
)

// Packed represents one data point measuring the cost counter's value over
// time.  The cost counter is a monotonically increasing value which records
// the sum of all costs incurred up to a moment in time.
//
// Unlike Sample, time is recorded as a delta relative to an epoch.
//
// ~~ RATIONALE ~~
//
// Go 1.9 (2017) introduced monotonic time support, which allows t1.Sub(t2) to
// return a meaningful value even when the wallclock time has changed between
// t1 and t2, assuming both t1 and t2 were obtained with time.Now().  Other
// Time methods, such as Before and After, do *not* respect monotonic time;
// they look at the wallclock time *only*.  As we are going to be doing a lot
// of Before/After-like comparisons between timestamps, it's best for us
// convert all Time values to the same time base to better deal with leap
// seconds, NTP time adjustments, and other such complications with using
// wallclock time.
//
// We do this by defining an arbitrary Time as our epoch, obtaining it via
// time.Now() and converting all other Time values into Durations relative to
// it, and then doing all operations on Durations instead of on Times.  We only
// convert back to Times when exporting data to another node, as monotonic
// times cannot be exported from the machine on which they live: monotonic
// times are measured as seconds elapsed since boot in most OSes, which means
// each node's monotonic counter has a different epoch, and wallclock time is
// the only way for nodes to interoperate with consistency.
//
// This scheme has a minor bonus benefit: Durations are significantly smaller
// than Times, because Times maintain two 64-bit timestamps and a 32/64-bit
// timezone pointer while Durations are a single 64-bit value.  This helps a
// bit when dealing with lots of samples, since it means that sizeof(Packed) is
// about half of sizeof(Sample).
//
// References:
//
// - https://golang.org/doc/go1.9#monotonic-time
//
// - https://go.googlesource.com/proposal/+/master/design/12914-monotonic.md
//
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
