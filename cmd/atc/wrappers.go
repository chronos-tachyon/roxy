package main

import (
	"sort"
)

type ServiceName string

type ShardNumber uint32

type Location string

type GraphID uint32

type LocationAndCost struct {
	Location Location
	Cost     float32
}

// type ServiceNameList {{{

type ServiceNameList []ServiceName

func (list ServiceNameList) Len() int {
	return len(list)
}

func (list ServiceNameList) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (list ServiceNameList) Less(i, j int) bool {
	a, b := list[i], list[j]
	return a < b
}

func (list ServiceNameList) Sort() {
	sort.Sort(list)
}

func (list ServiceNameList) Search(item ServiceName) (index uint, found bool) {
	length := uint(len(list))
	index = uint(sort.Search(int(length), func(i int) bool {
		return list[i] >= item
	}))
	found = (index < length && list[index] == item)
	return
}

var _ sort.Interface = ServiceNameList(nil)

// }}}

// type LocationAndCostList {{{

type LocationAndCostList []LocationAndCost

func (list LocationAndCostList) Len() int {
	return len(list)
}

func (list LocationAndCostList) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (list LocationAndCostList) Less(i, j int) bool {
	a, b := list[i], list[j]
	if a.Cost != b.Cost {
		return a.Cost < b.Cost
	}
	return a.Location < b.Location
}

func (list LocationAndCostList) Sort() {
	sort.Sort(list)
}

var _ sort.Interface = LocationAndCostList(nil)

// }}}
