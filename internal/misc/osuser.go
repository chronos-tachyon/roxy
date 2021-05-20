package misc

import (
	"os/user"
	"sync"

	"github.com/chronos-tachyon/roxy/lib/roxyutil"
)

// LookupCache is a thread-safe cache that remembers the results of OS user and
// group lookups.
type LookupCache struct {
	mu          sync.Mutex
	userByID    map[uint32]*userCacheEntry
	userByName  map[string]*userCacheEntry
	groupByID   map[uint32]*groupCacheEntry
	groupByName map[string]*groupCacheEntry
}

type userCacheEntry struct {
	cv    *sync.Cond
	u     *user.User
	err   error
	ready bool
}

type groupCacheEntry struct {
	cv    *sync.Cond
	g     *user.Group
	err   error
	ready bool
}

// UserByID is a cached wrapper of "os/user".LookupId.
func (cache *LookupCache) UserByID(uid uint32) (*user.User, error) {
	cache.mu.Lock()
	if cache.userByID == nil {
		cache.userByID = make(map[uint32]*userCacheEntry, 32)
	}
	entry := cache.userByID[uid]
	if entry == nil {
		entry = new(userCacheEntry)
		entry.cv = sync.NewCond(&cache.mu)
		cache.userByID[uid] = entry
		cache.mu.Unlock()
		u, err := roxyutil.LookupUserByID(uid)
		cache.mu.Lock()
		entry.u = u
		entry.err = err
		entry.ready = true
		entry.cv.Broadcast()
	} else {
		for !entry.ready {
			entry.cv.Wait()
		}
	}
	cache.mu.Unlock()
	return entry.u, entry.err
}

// UserByName is a cached wrapper of "os/user".Lookup.
func (cache *LookupCache) UserByName(userName string) (*user.User, error) {
	cache.mu.Lock()
	if cache.userByName == nil {
		cache.userByName = make(map[string]*userCacheEntry, 32)
	}
	entry := cache.userByName[userName]
	if entry == nil {
		entry = new(userCacheEntry)
		entry.cv = sync.NewCond(&cache.mu)
		cache.userByName[userName] = entry
		cache.mu.Unlock()
		u, err := roxyutil.LookupUserByName(userName)
		cache.mu.Lock()
		entry.u = u
		entry.err = err
		entry.ready = true
		entry.cv.Broadcast()
	} else {
		for !entry.ready {
			entry.cv.Wait()
		}
	}
	cache.mu.Unlock()
	return entry.u, entry.err
}

// GroupByID is a cached wrapper of "os/user".LookupGroupId.
func (cache *LookupCache) GroupByID(gid uint32) (*user.Group, error) {
	cache.mu.Lock()
	if cache.groupByID == nil {
		cache.groupByID = make(map[uint32]*groupCacheEntry, 32)
	}
	entry := cache.groupByID[gid]
	if entry == nil {
		entry = new(groupCacheEntry)
		entry.cv = sync.NewCond(&cache.mu)
		cache.groupByID[gid] = entry
		cache.mu.Unlock()
		g, err := roxyutil.LookupGroupByID(gid)
		cache.mu.Lock()
		entry.g = g
		entry.err = err
		entry.ready = true
		entry.cv.Broadcast()
	} else {
		for !entry.ready {
			entry.cv.Wait()
		}
	}
	cache.mu.Unlock()
	return entry.g, entry.err
}

// GroupByName is a cached wrapper of "os/user".LookupGroup.
func (cache *LookupCache) GroupByName(groupName string) (*user.Group, error) {
	cache.mu.Lock()
	if cache.groupByName == nil {
		cache.groupByName = make(map[string]*groupCacheEntry, 32)
	}
	entry := cache.groupByName[groupName]
	if entry == nil {
		entry = new(groupCacheEntry)
		entry.cv = sync.NewCond(&cache.mu)
		cache.groupByName[groupName] = entry
		cache.mu.Unlock()
		g, err := roxyutil.LookupGroupByName(groupName)
		cache.mu.Lock()
		entry.g = g
		entry.err = err
		entry.ready = true
		entry.cv.Broadcast()
	} else {
		for !entry.ready {
			entry.cv.Wait()
		}
	}
	cache.mu.Unlock()
	return entry.g, entry.err
}
