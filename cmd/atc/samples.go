package main

const (
	// NumSamples is the number of samples to store.
	//
	// 60 samples = (10 minutes) * (60 seconds / minute) * (1 sample / 10 seconds).
	NumSamples = 60

	// DecayFactor is the relative weight of old samples vs new samples.
	//
	// 0.875 = 7/8 = (weight of sample N-1) / (weight of sample N).
	DecayFactor = 0.875
)

type SampleHistory struct {
	raw     [NumSamples]float32
	data    []float32
	average float32
}

func (history *SampleHistory) Clear() {
	history.data = nil
	history.average = 0.0
}

func (history *SampleHistory) Average() float32 {
	return history.average
}

func (history *SampleHistory) Add(sample float32) {
	n := uint(len(history.data))

	if n == 0 {
		history.raw[0] = sample
		history.data = history.raw[0:1]
		history.average = sample
		return
	}

	if n < NumSamples {
		history.raw[n] = sample
		n++
		history.data = history.raw[0:n]

		var numerator, denominator float64
		for i := uint(1); i <= n; i++ {
			current := history.data[n-i]
			weight := sampleWeights[NumSamples-i]
			numerator += weight * float64(current)
			denominator += weight
		}
		history.average = float32(numerator / denominator)
		return
	}

	for i := uint(0); i < NumSamples-1; i++ {
		history.raw[i] = history.raw[i+1]
	}
	history.raw[NumSamples-1] = sample
	history.data = history.raw[:]

	var sum float64
	for i := uint(NumSamples); i > 0; i-- {
		current := history.data[i-1]
		weight := sampleWeights[i-1]
		sum += weight * float64(current)
	}
	history.average = float32(sum)
}

var sampleWeights [NumSamples]float64

func init() {
	n := uint(NumSamples) - 1

	var sum float64 = 1.0
	sampleWeights[n] = 1.0

	for i := n; i > 0; i-- {
		nextWeight := sampleWeights[i] * DecayFactor
		sum += nextWeight
		sampleWeights[i-1] = nextWeight
	}

	norm := 1.0 / sum

	for i := uint(0); i < NumSamples; i++ {
		sampleWeights[i] *= norm
	}
}
