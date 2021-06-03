package picker

import (
	"math"
	"math/rand"

	"github.com/chronos-tachyon/roxy/lib/syncrand"
)

// Picker is a utility class for picking values from a list using weighted
// random selection.  The weight is based on a user-defined score function,
// where low scores are exponentially more favorable than high scores.
type Picker struct {
	values  []interface{}
	scores  []float64
	weights []float64
	probs   []float64
	cprobs  []float64
}

// Make creates a Picker with the given values and score function.
func Make(values []interface{}, scoreFn func(interface{}) float64) Picker {
	p := Picker{
		values:  make([]interface{}, len(values)),
		scores:  make([]float64, len(values)),
		weights: make([]float64, len(values)),
		probs:   make([]float64, len(values)),
		cprobs:  make([]float64, len(values)),
	}

	for index, value := range values {
		score := scoreFn(value)
		weight := math.Exp2(-score)
		p.values[index] = value
		p.scores[index] = score
		p.weights[index] = weight
	}

	p.recomputeProbabilities()

	return p
}

// Len returns the number of values in this Picker.
func (p Picker) Len() uint {
	return uint(len(p.values))
}

// Get returns the index'th value in this Picker, using 0-based indexing.
func (p Picker) Get(index uint) interface{} {
	return p.values[index]
}

// Disable semi-permanently alters the probability of selecting the index'th
// value to 0.
func (p Picker) Disable(index uint) {
	p.weights[index] = 0.0
	p.recomputeProbabilities()
}

// Enable undoes the effect of a previous call to Disable.
func (p Picker) Enable(index uint) {
	p.weights[index] = math.Exp2(-p.scores[index])
	p.recomputeProbabilities()
}

// Worst deterministically picks the index of the value with the worst score.
// Disabled values are ignored.
func (p Picker) Worst() (uint, bool) {
	var (
		hasWorst   bool
		worstIndex uint
		worstProb  float64
	)
	length := p.Len()
	for index := uint(0); index < length; index++ {
		prob := p.probs[index]
		if prob == 0.0 {
			continue
		}
		if hasWorst && prob >= worstProb {
			continue
		}
		hasWorst = true
		worstIndex = index
		worstProb = prob
	}
	return worstIndex, hasWorst
}

// Pick randomly selects the index of a value.
func (p Picker) Pick(rng *rand.Rand) uint {
	if rng == nil {
		rng = syncrand.Global()
	}

	k := rng.Float64()
	length := p.Len()
	index := uint(0)
	for {
		if k < p.cprobs[index] {
			break
		}
		index++
		if index == length {
			index--
			break
		}
	}
	return index
}

// Divvy divides and distributes the given quantity among the values according
// to their individual probabilities.
func (p Picker) Divvy(amount float64) []float64 {
	length := p.Len()
	out := make([]float64, length)
	for index := uint(0); index < length; index++ {
		out[index] = amount * p.probs[index]
	}
	return out
}

func (p Picker) recomputeProbabilities() {
	length := p.Len()

	var sumOfWeights float64
	for index := uint(0); index < length; index++ {
		sumOfWeights += p.weights[index]
	}
	if sumOfWeights == 0.0 {
		sumOfWeights = 1.0
	}

	norm := 1.0 / sumOfWeights
	var cumulativeProbability float64
	for index := uint(0); index < length; index++ {
		weight := p.weights[index]
		prob := weight * norm
		cumulativeProbability += prob
		p.probs[index] = prob
		p.cprobs[index] = cumulativeProbability
	}
}
