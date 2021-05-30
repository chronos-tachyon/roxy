package costhistory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// CostHistory records the history of a cost-per-second (CPS) metric, where CPS
// is defined as queries-per-second (QPS) times a cost factor per query.
//
// It is designed to compute the "average" CPS over a window of time.  The
// detailed algorithm is subject to change, but currently: "average" is defined
// by bucketing samples by age and computing an exponentially-decaying weighted
// average of the pairwise bucket-by-bucket QPS, with each QPS weighted by a
// function of the bucket age.  In simple terms, data for the most recent
// sample is weighted most heavily, data from the oldest known sample is
// weighted least heavily, and data from before the start of the window is
// forgotten entirely.
//
// CostHistory is thread-safe.
type CostHistory struct {
	NumBuckets     uint
	BucketInterval time.Duration
	DecayFactor    float64
	NowFn          func() time.Time
	Mutex          sync.Locker

	buckets      []uint64
	samples      []Packed
	epoch        time.Time
	timeDeltaNow time.Duration
	counterBias  uint64
	counterThen  uint64
	counterNow   uint64
	perSecond    float64
}

// Init initializes a CostHistory.
func (hist *CostHistory) Init() {
	if hist.NumBuckets == 0 {
		hist.NumBuckets = 60
	}
	if hist.BucketInterval == 0 {
		hist.BucketInterval = 1 * time.Second
	}
	if hist.DecayFactor == 0.0 {
		hist.DecayFactor = 0.984375 // 63/64
	}
	if hist.NowFn == nil {
		hist.NowFn = time.Now
	}
	if hist.Mutex == nil {
		hist.Mutex = dummyLocker{}
	}
	hist.buckets = make([]uint64, hist.NumBuckets)
	hist.samples = make([]Packed, 0, hist.NumBuckets)
	hist.epoch = hist.Now()
	hist.timeDeltaNow = 0
	hist.counterBias = 0
	hist.counterThen = 0
	hist.counterNow = 0
	hist.perSecond = 0.0
}

// WindowInterval returns the calculated width of the sample window.
func (hist *CostHistory) WindowInterval() time.Duration {
	return time.Duration(hist.NumBuckets) * hist.BucketInterval
}

// Now returns the current time in UTC.
func (hist *CostHistory) Now() time.Time {
	return hist.NowFn().UTC()
}

// Reset resets the CostHistory to cost counter 0 and no sample data.
func (hist *CostHistory) Reset() {
	hist.Mutex.Lock()
	hist.buckets = make([]uint64, hist.NumBuckets)
	hist.samples = make([]Packed, 0, hist.NumBuckets)
	hist.epoch = hist.Now()
	hist.timeDeltaNow = 0
	hist.counterBias = 0
	hist.counterThen = 0
	hist.counterNow = 0
	hist.perSecond = 0.0
	hist.Mutex.Unlock()
}

// Data obtains a Data snapshot.
func (hist *CostHistory) Data() Data {
	hist.Mutex.Lock()
	out := Data{
		Now:       hist.epoch.Add(hist.timeDeltaNow),
		Counter:   hist.counterNow,
		PerSecond: hist.perSecond,
	}
	hist.Mutex.Unlock()

	return out
}

// Snapshot obtains a copy of the detailed contents of the CostHistory's sample
// time window.  The first Sample represents the time and absolute value of the
// cost counter at the start of the window, and each subsequent Sample records
// (a) when the counter changed and (b) what value it changed to.  The current
// cost counter is the value of Sample.Counter for the last Sample.
func (hist *CostHistory) Snapshot() []Sample {
	windowInterval := hist.WindowInterval()

	hist.Mutex.Lock()

	lengthIn := uint(len(hist.samples))
	now := hist.epoch.Add(hist.timeDeltaNow)
	then := now.Add(-windowInterval)

	needFinalSample := false
	lengthOut := 1 + lengthIn
	if lengthIn == 0 || hist.samples[lengthIn-1].Delta != hist.timeDeltaNow {
		needFinalSample = true
		lengthOut++
	}

	samples := make([]Sample, lengthOut)
	samples[0] = Sample{Time: then, Counter: hist.counterThen}
	for index := uint(0); index < lengthIn; index++ {
		samples[index+1] = hist.samples[index].Unpack(hist.epoch)
	}
	if needFinalSample {
		samples[lengthIn+1] = Sample{Time: now, Counter: hist.counterNow}
	}

	hist.Mutex.Unlock()

	return samples
}

// Update updates the sample window and recomputes the average without adding a
// new sample.
func (hist *CostHistory) Update() {
	hist.Mutex.Lock()
	hist.timeDeltaNow = hist.Now().Sub(hist.epoch)
	hist.expireOldSamples()
	hist.regenerateBuckets()
	hist.updateAverage()
	hist.Mutex.Unlock()
}

// UpdateRelative creates a new sample with the given (non-negative) cost delta
// added to the current cost counter, in addition to updating the sample window
// and recomputing the average.
func (hist *CostHistory) UpdateRelative(counterDelta uint64) {
	hist.Mutex.Lock()
	timeDeltaNow := hist.Now().Sub(hist.epoch)
	counterAbs := hist.counterNow + counterDelta
	hist.timeDeltaNow = timeDeltaNow
	hist.expireOldSamples()
	hist.appendSample(timeDeltaNow, counterAbs)
	hist.regenerateBuckets()
	hist.updateAverage()
	hist.Mutex.Unlock()
}

// UpdateAbsolute creates a new sample with the given (larger) counter value,
// in addition to updating the sample window and recomputing the average.
//
// If the new cost counter is less than the previous cost counter, it's assumed
// that a reset-to-zero has occurred, and the previous cost counter will be
// added to this and all future absolute cost counter values.
func (hist *CostHistory) UpdateAbsolute(counter uint64) {
	hist.Mutex.Lock()
	timeDeltaNow := hist.Now().Sub(hist.epoch)
	counterAbs := counter + hist.counterBias
	if counterAbs < hist.counterNow {
		hist.counterBias = hist.counterNow
		counterAbs = counter + hist.counterBias
	}
	hist.timeDeltaNow = timeDeltaNow
	hist.expireOldSamples()
	hist.appendSample(timeDeltaNow, counterAbs)
	hist.regenerateBuckets()
	hist.updateAverage()
	hist.Mutex.Unlock()
}

// BulkAppend inserts the given samples into the time window, recomputing the
// average CPS.  In addition to being more efficient than calling
// UpdateRelative or UpdateAbsolute repeatedly, it also allows the caller to
// specify the collection time of each sample.
func (hist *CostHistory) BulkAppend(samples ...Sample) {
	newLength := uint(len(samples))

	if newLength == 0 {
		return
	}

	hist.Mutex.Lock()
	defer hist.Mutex.Unlock()

	packedList := PackSamples(hist.epoch, samples)

	prevDelta := packedList[0].Delta - time.Nanosecond
	prevCounter := packedList[0].Counter
	for index := uint(0); index < newLength; index++ {
		packed := packedList[index]
		roxyutil.Assertf(
			packed.Delta > prevDelta,
			"samples go backward in time: %q vs %q",
			hist.epoch.Add(prevDelta).Format(time.RFC3339Nano),
			hist.epoch.Add(packed.Delta).Format(time.RFC3339Nano),
		)
		roxyutil.Assertf(
			packed.Counter >= prevCounter,
			"samples go backward in value: %d vs %d",
			prevCounter,
			packed.Counter,
		)
		prevDelta, prevCounter = packed.Delta, packed.Counter
	}

	d0 := packedList[newLength-1].Delta
	d1 := packedList[0].Delta

	// Throw away all old samples Delta <= (d0 - WindowInterval)
	hist.timeDeltaNow = d0
	hist.expireOldSamples()

	// Throw away all old samples Delta >= d1
	index := indexPred(hist.samples, func(packed Packed) bool {
		return packed.Delta >= d1
	})
	if length := uint(len(hist.samples)); index < length {
		hist.samples = trimEnd(hist.samples, length-index)
	}

	// Copy all remaining old samples to a large enough list
	oldLength := uint(len(hist.samples))
	length := oldLength + newLength
	list := hist.samples
	if length > uint(cap(hist.samples)) {
		list = make([]Packed, oldLength, length)
		copy(list, hist.samples)
	}

	// Compute the counterBias for the new samples
	var counterBias uint64
	if hist.counterNow > packedList[0].Counter {
		counterBias = hist.counterNow
	}

	// Append the new samples to the list
	for index := uint(0); index < newLength; index++ {
		packed := packedList[index]
		packed.Counter += counterBias
		list = append(list, packed)
	}
	hist.samples = list
	hist.counterBias = counterBias
	hist.counterNow = prevCounter

	// Do a second expire pass, for any expired new samples
	hist.expireOldSamples()
	hist.regenerateBuckets()
	hist.updateAverage()
}

func (hist *CostHistory) expireOldSamples() {
	timeDeltaLimit := hist.timeDeltaNow - hist.WindowInterval()

	index := indexPred(hist.samples, func(packed Packed) bool {
		return packed.Delta > timeDeltaLimit
	})

	var s Packed
	var ok bool
	s, hist.samples, ok = trimBegin(hist.samples, index)
	if ok {
		hist.counterThen = s.Counter
	}
}

func (hist *CostHistory) appendSample(timeDelta time.Duration, counterAbs uint64) {
	if counterAbs == hist.counterNow {
		return
	}

	hist.counterNow = counterAbs
	hist.samples = append(hist.samples, Packed{timeDelta, counterAbs})
}

func (hist *CostHistory) regenerateBuckets() {
	for index := uint(0); index < hist.NumBuckets; index++ {
		hist.buckets[index] = hist.counterThen
	}

	for _, curr := range hist.samples {
		timeDeltaCurrNow := hist.timeDeltaNow - curr.Delta
		bucketIndex := uint(math.Floor(float64(timeDeltaCurrNow) / float64(hist.BucketInterval)))
		bucketIndex = hist.NumBuckets - (bucketIndex + 1)
		if hist.buckets[bucketIndex] < curr.Counter {
			hist.buckets[bucketIndex] = curr.Counter
		}
	}

	for index := uint(1); index < hist.NumBuckets; index++ {
		prev := hist.buckets[index-1]
		curr := hist.buckets[index]
		if prev > curr {
			hist.buckets[index] = prev
		}
	}
}

func (hist *CostHistory) updateAverage() {
	var sumOfAverages float64
	var sumOfWeights float64
	for index := uint(0); index < hist.NumBuckets; index++ {
		prev := hist.counterThen
		if index != 0 {
			prev = hist.buckets[index-1]
		}
		curr := hist.buckets[index]
		counterDelta := curr - prev
		avgPerSecond := float64(counterDelta) * float64(time.Second) / float64(hist.BucketInterval)

		ageTicks := float64(hist.NumBuckets - index)
		weight := math.Pow(hist.DecayFactor, ageTicks)

		sumOfAverages += avgPerSecond * weight
		sumOfWeights += weight
	}
	hist.perSecond = sumOfAverages / sumOfWeights
}

// Debug writes a debugging dump to the provided io.Writer.  No stability
// guarantees are provided for the output format.
func (hist *CostHistory) Debug(w io.Writer) {
	buckets := make([]string, hist.NumBuckets)
	for index := uint(0); index < hist.NumBuckets; index++ {
		buckets[index] = strconv.FormatUint(hist.buckets[index], 10)
	}

	length := uint(len(hist.samples))
	samples := make([]string, length)
	for index := uint(0); index < length; index++ {
		s := hist.samples[index]
		samples[index] = s.Delta.String() + "/" + strconv.FormatUint(s.Counter, 10)
	}

	m := map[string]interface{}{
		"NumBuckets":     hist.NumBuckets,
		"BucketInterval": hist.BucketInterval.String(),
		"DecayFactor":    json.Number(fmt.Sprintf("%f", hist.DecayFactor)),
		"buckets":        "[" + strings.Join(buckets, " ") + "]",
		"samples":        "[" + strings.Join(samples, " ") + "]",
		"epoch":          hist.epoch.Format(time.RFC3339Nano),
		"timeDeltaNow":   hist.timeDeltaNow.String(),
		"counterBias":    hist.counterBias,
		"counterThen":    hist.counterThen,
		"counterNow":     hist.counterNow,
		"perSecond":      json.Number(fmt.Sprintf("%f", hist.perSecond)),
	}

	var buf bytes.Buffer
	buf.Grow(256)

	e := json.NewEncoder(&buf)
	e.SetEscapeHTML(false)
	e.SetIndent("", "  ")

	err := e.Encode(m)
	if err != nil {
		panic(err)
	}

	_, err = buf.WriteTo(w)
	if err != nil {
		panic(err)
	}
}

func trimBegin(list []Packed, n uint) (Packed, []Packed, bool) {
	if n == 0 {
		return Packed{}, list, false
	}

	oldLength := uint(len(list))
	newLength := oldLength - n
	s := list[n-1]
	for index := uint(0); index < newLength; index++ {
		list[index] = list[index+n]
	}
	for index := newLength; index < oldLength; index++ {
		list[index] = Packed{}
	}
	return s, list[:newLength], true
}

func trimEnd(list []Packed, n uint) []Packed {
	if n == 0 {
		return list
	}

	oldLength := uint(len(list))
	newLength := oldLength - n
	for index := newLength; index < oldLength; index++ {
		list[index] = Packed{}
	}
	return list[:newLength]
}

func indexPred(list []Packed, pred func(Packed) bool) uint {
	length := uint(len(list))
	for index := uint(0); index < length; index++ {
		if pred(list[index]) {
			return index
		}
	}
	return length
}

type dummyLocker struct{}

func (dummyLocker) Lock()   {}
func (dummyLocker) Unlock() {}

var _ sync.Locker = dummyLocker{}
