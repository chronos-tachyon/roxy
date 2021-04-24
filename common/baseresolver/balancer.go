package baseresolver

import (
	"fmt"
	"math/rand"
	"sync"

	multierror "github.com/hashicorp/go-multierror"
)

const (
	minLoad   = 1.0 / float32(1024.0)
	maxLoad   = float32(1024.0)
	minWeight = 1.0 / float32(65536.0)
	maxWeight = float32(65536.0)
)

func balanceImpl(balancer BalancerType, errs multierror.Error, resolved []*AddrData, rng *rand.Rand, perm []int, mu *sync.Mutex, nextRR *uint) (*AddrData, error) {
	if len(resolved) == 0 {
		if err := errs.ErrorOrNil(); err != nil {
			return nil, err
		}
		return nil, ErrNoHealthyBackends
	}

	switch balancer {
	case RandomBalancer:
		candidates := make([]*AddrData, 0, len(resolved))
		for _, data := range resolved {
			if data.IsHealthy() {
				candidates = append(candidates, data)
			}
		}
		if len(candidates) != 0 {
			index := rng.Intn(len(candidates))
			return candidates[index], nil
		}

	case RoundRobinBalancer:
		mu.Lock()
		defer mu.Unlock()

		for tries := uint(len(perm)); tries > 0; tries-- {
			k := *nextRR
			*nextRR = (*nextRR + 1) % uint(len(perm))
			data := resolved[perm[k]]
			if data.IsHealthy() {
				return data, nil
			}
		}

	case LeastLoadedBalancer:
		candidates := make([]*AddrData, 0, len(resolved))
		for _, data := range resolved {
			if data.IsHealthy() {
				candidates = append(candidates, data)
			}
		}
		if len(candidates) != 0 {
			weights := make([]float32, len(candidates))
			var sumOfWeights float32
			for index, data := range candidates {
				load, _ := data.GetLoad()
				if load < minLoad {
					load = minLoad
				}
				if load > maxLoad {
					load = maxLoad
				}
				weight := 1.0 / load
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

			k := rng.Float32()
			for index, boundary := range cumulative {
				if k < boundary {
					return candidates[index], nil
				}
			}

			index := rng.Intn(len(candidates))
			return candidates[index], nil
		}

	case SRVBalancer:
		start := 0
		length := len(resolved)
		for start < length {
			data := resolved[start]
			if data.IsHealthy() {
				break
			}
		}
		if start < length {
			a := resolved[start]
			candidates := make([]*AddrData, 0, length-start)
			candidates = append(candidates, a)
			for end := start + 1; end < length; end++ {
				b := resolved[end]
				if *a.SRVPriority != *b.SRVPriority {
					break
				}
				if b.IsHealthy() {
					candidates = append(candidates, b)
				}
			}

			if len(candidates) == 1 {
				return candidates[0], nil
			}

			weights := make([]float32, len(candidates))
			var sumOfWeights float32
			for index, data := range candidates {
				weight := *data.ComputedWeight
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

			k := rng.Float32()
			for index, boundary := range cumulative {
				if k < boundary {
					return candidates[index], nil
				}
			}

			index := rng.Intn(len(candidates))
			return candidates[index], nil
		}

	default:
		panic(fmt.Errorf("%#v not implemented", balancer))
	}

	if err := errs.ErrorOrNil(); err != nil {
		return nil, err
	}

	return nil, ErrNoHealthyBackends
}
