package proxy

import (
	"cmp"
	"net/netip"
	"slices"
	"sync"
	"time"
)

const maxTrackedAddrs = 1000

// Access summarizes the requests one client address has made.
type Access struct {
	Addr     netip.Addr
	Allowed  bool
	Requests int
	LastSeen time.Time
}

type accessLog struct {
	mu        sync.Mutex
	byAddr    map[netip.Addr]Access
	untracked int
}

func (l *accessLog) record(addr netip.Addr, allowed bool, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	access, tracked := l.byAddr[addr]
	if !tracked && len(l.byAddr) >= maxTrackedAddrs {
		l.untracked++
		return
	}
	if l.byAddr == nil {
		l.byAddr = make(map[netip.Addr]Access)
	}
	access.Addr = addr
	access.Allowed = allowed
	access.Requests++
	access.LastSeen = now
	l.byAddr[addr] = access
}

func (l *accessLog) snapshot() (accesses []Access, untracked int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	accesses = make([]Access, 0, len(l.byAddr))
	for _, access := range l.byAddr {
		accesses = append(accesses, access)
	}
	slices.SortFunc(accesses, func(a, b Access) int {
		if byTime := b.LastSeen.Compare(a.LastSeen); byTime != 0 {
			return byTime
		}
		return cmp.Compare(a.Addr.String(), b.Addr.String())
	})
	return accesses, l.untracked
}
