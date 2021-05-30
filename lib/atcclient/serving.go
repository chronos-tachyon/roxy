package atcclient

import (
	"sync"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// WatchID tracks a registered WatchFunc.
type WatchID uint32

// WatchFunc is the callback type for observing changes in serving status.
type WatchFunc func(bool)

var (
	gServingMutex sync.Mutex
	gIsServing    bool
	gLastWatchID  WatchID
	gWatches      map[WatchID]WatchFunc
)

// IsServing returns true if the ATC client is currently serving.
//
// For a server, "serving" means accepting inbound traffic.  For a client,
// "serving" means actively generating non-trivial outbound traffic.
func IsServing() bool {
	gServingMutex.Lock()
	isServing := gIsServing
	gServingMutex.Unlock()
	return isServing
}

// SetIsServing modifies the serving status.
func SetIsServing(isServing bool) {
	gServingMutex.Lock()
	defer gServingMutex.Unlock()

	if gIsServing == isServing {
		return
	}

	gIsServing = isServing
	for _, fn := range gWatches {
		fn(isServing)
	}
}

// WatchIsServing calls the given callback when the serving status changes.
func WatchIsServing(fn WatchFunc) WatchID {
	roxyutil.AssertNotNil(&fn)

	gServingMutex.Lock()
	if gWatches == nil {
		gWatches = make(map[WatchID]WatchFunc, 4)
	}
	gLastWatchID++
	id := gLastWatchID
	gWatches[id] = fn
	fn(gIsServing)
	gServingMutex.Unlock()

	return id
}

// CancelWatchIsServing cancels a previous call to WatchIsServing.
func CancelWatchIsServing(id WatchID) {
	gServingMutex.Lock()
	delete(gWatches, id)
	gServingMutex.Unlock()
}
