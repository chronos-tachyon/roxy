package roxyresolver

import (
	"fmt"
	"math"
	"math/rand"
	"sync/atomic"

	multierror "github.com/hashicorp/go-multierror"
)

func computePermImpl(bal BalancerType, resolved []Resolved, rng *rand.Rand) []int {
	switch bal {
	case RoundRobinBalancer:
		return rng.Perm(len(resolved))

	case WeightedRoundRobinBalancer:
		cumulative := computeCumulativeProbabilities(resolved, standardWeightFn)
		const length = 256
		perm := make([]int, length)
		j := 0
		for i := range cumulative {
			probability := cumulative[i]
			if i > 0 {
				probability -= cumulative[i-1]
			}
			n := int(math.RoundToEven(length * float64(probability)))
			k := j + n
			if k > length {
				k = length
			}
			for ; j < k; j++ {
				perm[j] = i
			}
		}
		for ; j < length; j++ {
			perm[j] = len(cumulative) - 1
		}
		rng.Shuffle(length, func(ii, jj int) {
			perm[ii], perm[jj] = perm[jj], perm[ii]
		})
		return perm

	default:
		return nil
	}
}

func balanceImpl(bal BalancerType, errs multierror.Error, resolved []Resolved, rng *rand.Rand, perm []int, nextRR *uint32) (Resolved, error) {
	if len(resolved) == 0 {
		err := errs.ErrorOrNil()
		if err == nil {
			err = ErrNoHealthyBackends
		}
		return Resolved{}, err
	}

	switch bal {
	case RandomBalancer:
		candidates := findHealthyCandidates(resolved)
		if data, ok := pickUniformRandom(candidates, rng); ok {
			return data, nil
		}

	case LeastLoadedBalancer:
		candidates := findHealthyCandidates(resolved)
		if data, ok := pickWeightedRandom(candidates, rng, loadWeightFn); ok {
			return data, nil
		}

	case SRVBalancer:
		start := 0
		length := len(resolved)
		for ; start < length; start++ {
			data := resolved[start]
			if data.IsHealthy() {
				break
			}
		}
		if start < length {
			a := resolved[start]
			candidates := make([]Resolved, 1, length-start)
			candidates[0] = a
			for end := start + 1; end < length; end++ {
				b := resolved[end]
				if a.SRVPriority != b.SRVPriority {
					break
				}
				if b.IsHealthy() {
					candidates = append(candidates, b)
				}
			}
			if data, ok := pickWeightedRandom(candidates, rng, standardWeightFn); ok {
				return data, nil
			}
		}

	case WeightedRandomBalancer:
		candidates := findHealthyCandidates(resolved)
		if data, ok := pickWeightedRandom(candidates, rng, standardWeightFn); ok {
			return data, nil
		}

	case RoundRobinBalancer:
		fallthrough
	case WeightedRoundRobinBalancer:
		length := uint32(len(perm))
		for tries := length; tries > 0; tries-- {
			k := (atomic.AddUint32(nextRR, 1) - 1) % length
			data := resolved[perm[k]]
			if data.IsHealthy() {
				return data, nil
			}
		}

	default:
		panic(fmt.Errorf("%#v not implemented", bal))
	}

	if err := errs.ErrorOrNil(); err != nil {
		return Resolved{}, err
	}

	return Resolved{}, ErrNoHealthyBackends
}

func findHealthyCandidates(resolved []Resolved) []Resolved {
	candidates := make([]Resolved, 0, len(resolved))
	for _, data := range resolved {
		if data.IsHealthy() {
			candidates = append(candidates, data)
		}
	}
	return candidates
}

var standardWeightFn = func(data Resolved) float32 {
	weight := data.Weight
	if weight < minWeight {
		weight = minWeight
	}
	if weight > maxWeight {
		weight = maxWeight
	}
	return weight
}

var loadWeightFn = func(data Resolved) float32 {
	load, _ := data.GetLoad()
	if load < minLoad {
		load = minLoad
	}
	if load > maxLoad {
		load = maxLoad
	}
	return 1.0 / load
}

func computeCumulativeProbabilities(candidates []Resolved, fn func(Resolved) float32) []float32 {
	weights := make([]float32, len(candidates))
	var sumOfWeights float32
	for index, data := range candidates {
		weight := fn(data)
		if weight < minWeight {
			weight = minWeight
		}
		if weight > maxWeight {
			weight = maxWeight
		}
		weights[index] = weight
		sumOfWeights += weight
	}

	norm := 1.0 / sumOfWeights
	cumulative := make([]float32, len(candidates))
	var partialSum float32
	for index, weight := range weights {
		probability := weight * norm
		partialSum += probability
		cumulative[index] = partialSum
	}

	return cumulative
}

func pickUniformRandom(candidates []Resolved, rng *rand.Rand) (Resolved, bool) {
	if len(candidates) == 0 {
		return Resolved{}, false
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	index := rng.Intn(len(candidates))
	return candidates[index], true
}

func pickWeightedRandom(candidates []Resolved, rng *rand.Rand, fn func(Resolved) float32) (Resolved, bool) {
	if len(candidates) == 0 {
		return Resolved{}, false
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}

	cumulative := computeCumulativeProbabilities(candidates, fn)
	k := rng.Float32()
	for index, boundary := range cumulative {
		if k < boundary {
			return candidates[index], true
		}
	}

	// fallback in case of bad floating point math
	index := len(candidates) - 1
	return candidates[index], true
}
