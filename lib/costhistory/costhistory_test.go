package costhistory

import (
	"math"
	"testing"
	"time"
)

const T0 = 1577836800

var expectPerSecond = []float64{
	0.00000000000000000000,
	0.50196078431372548323,
	0.75294117647058822484,
	0.87843137254901959565,
	0.94117647058823528106,
	0.97254901960784312376,
	0.98823529411764710062,
	0.99607843137254903354,
	1.00000000000000000000,
	0.49803921568627451677,
	0.24705882352941177516,
	0.12156862745098039047,
	0.05882352941176470507,
	0.02745098039215686236,
	0.01176470588235294101,
	0.00392156862745098034,
	0.00000000000000000000,
	0.00000000000000000000,
	0.00000000000000000000,
	0.00000000000000000000,
	0.00000000000000000000,
	0.00000000000000000000,
	0.00000000000000000000,
	0.00000000000000000000,
	0.00000000000000000000,
}

func TestCostHistory(t *testing.T) {
	t0 := time.Unix(T0, 0).UTC()
	tN := func(n uint) time.Time {
		dur := time.Duration(n) * time.Second
		return t0.Add(dur)
	}

	var nowCounter uint
	nowFn := func() time.Time {
		now := tN(nowCounter)
		nowCounter++
		return now
	}

	h := CostHistory{
		NumBuckets:     8,
		BucketInterval: 1 * time.Second,
		DecayFactor:    0.5,
		NowFn:          nowFn,
	}
	h.Init()

	data := h.Data()
	checkData(t, 0, data, Data{
		Now:       t0,
		Counter:   0,
		PerSecond: expectPerSecond[0],
	})

	for i := uint(1); i < 9; i++ {
		h.UpdateRelative(1)
		data := h.Data()
		checkData(t, i, data, Data{
			Now:       tN(i),
			Counter:   uint64(i),
			PerSecond: expectPerSecond[i],
		})
	}

	for i := uint(9); i < 17; i++ {
		h.Update()
		data := h.Data()
		checkData(t, i, data, Data{
			Now:       tN(i),
			Counter:   8,
			PerSecond: expectPerSecond[i],
		})
	}

	for i := uint(17); i < 25; i++ {
		h.Update()
		data := h.Data()
		checkData(t, i, data, Data{
			Now:       tN(i),
			Counter:   8,
			PerSecond: expectPerSecond[i],
		})
	}
}

func checkData(t *testing.T, index uint, actual Data, expect Data) {
	if !actual.Now.Equal(expect.Now) {
		t.Errorf(
			"[%d]: Now: expected %q, got %q",
			index,
			expect.Now.Format(time.RFC3339Nano),
			actual.Now.Format(time.RFC3339Nano),
		)
	}
	if actual.Counter != expect.Counter {
		t.Errorf(
			"[%d]: Counter: expected %d, got %d",
			index,
			expect.Counter,
			actual.Counter,
		)
	}
	if math.Abs(actual.PerSecond-expect.PerSecond) >= 1e-18 {
		t.Errorf(
			"[%d]: PerSecond: expected %.20f, got %.20f",
			index,
			expect.PerSecond,
			actual.PerSecond,
		)
	}
}
