// Copyright 2021 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package tracker

import (
	"container/list"
	"sync"
	"time"
)

// request tracks sent network requests which have not yet received a response.
type request struct {
	peer    string
	version uint // Protocol version

	reqCode uint64 // Protocol message code of the request
	resCode uint64 // Protocol message code of the expected response

	time   time.Time     // Timestamp when the request was made
	expire *list.Element // Expiration marker to untrack it
}

// Tracker is a pending network request tracker to measure how much time it takes
// a remote peer to respond.
type Tracker struct {
	protocol string        // Protocol capability identifier for the metrics
	timeout  time.Duration // Global timeout after which to drop a tracked packet

	pending map[uint64]*request // Currently pending requests
	expire  *list.List          // Linked list tracking the expiration order
	wake    *time.Timer         // Timer tracking the expiration of the next item

	lock sync.Mutex // Lock protecting from concurrent updates
}

// New creates a new network request tracker to monitor how much time it takes to
// fill certain requests and how individual peers perform.
func New(protocol string, timeout time.Duration) *Tracker {
	return &Tracker{
		protocol: protocol,
		timeout:  timeout,
		pending:  make(map[uint64]*request),
		expire:   list.New(),
	}
}

// clean is called automatically when a preset time passes without a response
// being delivered for the first network request.
func (t *Tracker) clean() {
	t.lock.Lock()
	defer t.lock.Unlock()

	// Expire anything within a certain threshold (might be no items at all if
	// we raced with the delivery)
	for t.expire.Len() > 0 {
		// Stop iterating if the next pending request is still alive
		var (
			head = t.expire.Front()
			id   = head.Value.(uint64)
			req  = t.pending[id]
		)
		if time.Since(req.time) < t.timeout+5*time.Millisecond {
			break
		}
		// Nope, dead, drop it
		t.expire.Remove(head)
		delete(t.pending, id)
	}
	t.schedule()
}

// schedule starts a timer to trigger on the expiration of the first network
// packet.
func (t *Tracker) schedule() {
	if t.expire.Len() == 0 {
		t.wake = nil
		return
	}
	t.wake = time.AfterFunc(time.Until(t.pending[t.expire.Front().Value.(uint64)].time.Add(t.timeout)), t.clean)
}
