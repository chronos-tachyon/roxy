package main

import (
	"fmt"
	"math"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

type CostMap struct {
	costByPair map[Location]map[Location]float32
	graphByLoc map[Location]GraphID
}

func NewCostMap(file CostFile) (*CostMap, error) {
	cm := new(CostMap)
	err := cm.compute(file)
	if err != nil {
		return nil, err
	}
	return cm, nil
}

func (cm *CostMap) IsKnown(loc Location) bool {
	_, found := cm.graphByLoc[loc]
	return found
}

func (cm *CostMap) GraphID(loc Location) (id GraphID, found bool) {
	id, found = cm.graphByLoc[loc]
	return
}

func (cm *CostMap) Connected(a, b Location) bool {
	aID, aFound := cm.graphByLoc[a]
	bID, bFound := cm.graphByLoc[b]
	return aFound && bFound && (aID == bID)
}

func (cm *CostMap) Cost(src Location, dst Location) (cost float32, connected bool) {
	connected = cm.Connected(src, dst)
	if connected && src != dst {
		cost = cm.costByPair[src][dst]
	} else if connected {
		cost = 0.0
	} else {
		cost = float32(math.Inf(1))
	}
	return
}

func (cm *CostMap) compute(file CostFile) error {
	// First, get a list of all unique locations.
	length, knownByPair, err := cm.phaseOne(file)
	if err != nil {
		return err
	}

	// Second, populate knownByPair with the explicit values from file.
	cm.phaseTwo(file, knownByPair)

	// Third, build the connectivity graph.
	cm.phaseThree(length, knownByPair)

	// Fourth, backfill costByPair with all {n^2 minus diagonal} entries.
	cm.phaseFour(length, knownByPair)

	return nil
}

func (cm *CostMap) phaseOne(
	file CostFile,
) (
	length uint,
	knownByPair map[Location]map[Location]float32,
	err error,
) {
	unique := make(map[Location]struct{}, len(file))
	for _, row := range file {
		err = roxyutil.ValidateATCLocation(row.A)
		if err != nil {
			err = fmt.Errorf("CostConfig[%q, %q, %f]: %w", row.A, row.B, row.Cost, err)
			return
		}

		err = roxyutil.ValidateATCLocation(row.B)
		if err != nil {
			err = fmt.Errorf("CostConfig[%q, %q, %f]: %w", row.A, row.B, row.Cost, err)
			return
		}

		if row.Cost < 0 {
			err = fmt.Errorf("CostConfig[%q, %q, %f]: negative cost", row.A, row.B, row.Cost)
			return
		}

		unique[Location(row.A)] = struct{}{}
		unique[Location(row.B)] = struct{}{}
	}

	length = uint(len(unique))

	cm.costByPair = make(map[Location]map[Location]float32, length)
	cm.graphByLoc = make(map[Location]GraphID, length)
	knownByPair = make(map[Location]map[Location]float32, length)
	for location := range unique {
		cm.costByPair[location] = make(map[Location]float32, length)
		knownByPair[location] = make(map[Location]float32, length)
	}
	return
}

func (cm *CostMap) phaseTwo(
	file CostFile,
	knownByPair map[Location]map[Location]float32,
) {
	for _, row := range file {
		a := Location(row.A)
		b := Location(row.B)
		if a == b {
			continue
		}
		knownByPair[a][b] = row.Cost
		knownByPair[b][a] = row.Cost
	}
}

func (cm *CostMap) phaseThree(
	length uint,
	knownByPair map[Location]map[Location]float32,
) {
	var lastGraphID GraphID
	for origin := range knownByPair {
		// Already marked as being part of a known graph? Skip.
		if _, found := cm.graphByLoc[origin]; found {
			continue
		}

		visited := make(map[Location]struct{}, length)
		queue := make([]Location, 0, length)

		// Allocate a new GraphID for this connected subgraph.
		lastGraphID++
		originGraphID := lastGraphID

		// Paint the origin.
		cm.graphByLoc[origin] = originGraphID
		visited[origin] = struct{}{}
		queue = append(queue, origin)

		// Paint everything new until there's nothing left to paint.
		for len(queue) != 0 {
			var current Location
			current, queue = queue[0], queue[1:]
			for next := range knownByPair[current] {
				if _, found := visited[next]; !found {
					cm.graphByLoc[next] = originGraphID
					visited[next] = struct{}{}
					queue = append(queue, next)
				}
			}
		}
	}
}

func (cm *CostMap) phaseFour(
	length uint,
	knownByPair map[Location]map[Location]float32,
) {
	for src, srcMap := range cm.costByPair {
		for dst, dstMap := range cm.costByPair {
			// Same location? Zero cost. Skip.
			if src == dst {
				continue
			}

			// Already populated? Best cost is already known. Skip.
			if _, found := srcMap[dst]; found {
				continue
			}

			// Not connected? Infinite cost. Skip.
			if !cm.Connected(src, dst) {
				continue
			}

			visited := make(map[Location]struct{}, length)
			visited[src] = struct{}{}

			queue := make(LocationAndCostList, 0, length)
			queue = append(queue, LocationAndCost{src, 0.0})

			best, hasBest := knownByPair[src][dst]

			for len(queue) != 0 {
				var current LocationAndCost
				current, queue = queue[0], queue[1:]

				currentMap := cm.costByPair[current.Location]
				if dstCost, found := currentMap[dst]; found {
					sum := current.Cost + dstCost

					// Shortcut: if we already have a better route,
					// don't consider any others for this node.
					//
					// (Negative costs are forbidden.)
					//
					// Note that currentMap[dst] is only populated
					// if it is known to be the *best* route from
					// current to dst.  We don't need to check any
					// other paths from current to dst.
					//
					if hasBest && sum >= best {
						continue
					}

					// Found a new best route candidate.
					hasBest = true
					best = sum
					continue
				}

				didAdd := false
				for next, nextCost := range currentMap {
					// Cycle detection: there is no way that visiting
					// next -> current -> ... -> next could be cheaper
					// than just visiting next once.
					//
					// (Negative costs are forbidden.)
					//
					if _, found := visited[next]; found {
						continue
					}

					sum := current.Cost + nextCost

					// Shortcut: if current -> next is at least as
					// big as our current best route to dst, then
					// there is no reason to consider any
					// possibility of current -> next -> ... -> dst.
					//
					// (Negative costs are forbidden.)
					//
					if hasBest && sum >= best {
						continue
					}

					visited[next] = struct{}{}
					queue = append(queue, LocationAndCost{next, sum})
					didAdd = true
				}

				// Make sure that we consider possibilities starting with
				// the cheapest possible route.  We're not doing a pure
				// greedy algorithm, but the sooner we can minimize best,
				// the more routes we can prune with our shortcuts.
				//
				if didAdd {
					queue.Sort()
				}
			}

			srcMap[dst] = best
			dstMap[src] = best
		}
	}
}
