package costhistory

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

// Sample represents one data point measuring the cost counter's value over
// time.  The cost counter is a monotonically increasing value which records
// the sum of all costs incurred up to a moment in time.
type Sample struct {
	Time    time.Time
	Counter uint64
}

// SampleFromProto converts roxy_v0.Sample to Sample.
func SampleFromProto(pb *roxy_v0.Sample) Sample {
	return Sample{
		Time:    pb.Timestamp.AsTime(),
		Counter: pb.Counter,
	}
}

// ToProto converts Sample to roxy_v0.Sample.
func (sample Sample) ToProto() *roxy_v0.Sample {
	return &roxy_v0.Sample{
		Timestamp: timestamppb.New(sample.Time),
		Counter:   sample.Counter,
	}
}

// SamplesFromProto converts []*roxy_v0.Sample to []Sample.
func SamplesFromProto(in []*roxy_v0.Sample) []Sample {
	out := make([]Sample, len(in))
	for index, pb := range in {
		out[index] = SampleFromProto(pb)
	}
	return out
}

// SamplesToProto converts []Sample to []*roxy_v0.Sample.
func SamplesToProto(in []Sample) []*roxy_v0.Sample {
	out := make([]*roxy_v0.Sample, len(in))
	for index, sample := range in {
		out[index] = sample.ToProto()
	}
	return out
}
