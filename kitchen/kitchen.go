// Package kitchen models a single fulfillment kitchen: three fixed-capacity
// storages (heater, cooler, shelf), the placement | move | discard algorithm
// and an indexed expiry heap so the discard for selection.
// Each Kitchen is self-contained and concurrency-safe thus can scale to many kitchens
// Dispatcher routes orders to kitchnes without any need for cross-kitchen coordination.
package kitchen

import (
	"io"
	"os"
	"sync"
	"time"

	"challenge/client"
)

const (
	HeaterCap = 6
	CoolerCap = 6
	ShelfCap  = 12
)

type Kitchen struct {
	mu     sync.Mutex
	heater *tempStore
	cooler *tempStore
	shelf  *shelf
	audit  *audit
	now    func() time.Time
	timers bool
}

type Option func(*Kitchen)

// WithOutput overrides the audit log destination.
// Default: os.Stdout. Pass io.Discard to silence the printer.
func WithOutput(w io.Writer) Option { return func(k *Kitchen) { k.audit.out = w } }

// WithClock overrides the time source for use by tests.
func WithClock(fn func() time.Time) Option { return func(k *Kitchen) { k.now = fn } }

// WithoutTimers disables real-time expiry timers.
// Tests can drive expiration manually via ExpireForTest.
func WithoutTimers() Option { return func(k *Kitchen) { k.timers = false } }

func New(opts ...Option) *Kitchen {
	now := time.Now()
	k := &Kitchen{
		heater: newTempStore(client.Heater, HeaterCap),
		cooler: newTempStore(client.Cooler, CoolerCap),
		shelf:  newShelf(ShelfCap),
		audit:  newAudit(os.Stdout, now),
		now:    time.Now,
		timers: true,
	}
	for _, opt := range opts {
		opt(k)
	}
	return k
}

// Inserts an order as follows:
//  1. Ideal storage if it has room,
//  2. Otherwise stores on the shelf if it has room,
//  3. Relocate an existing non-room shelf order back to its ideal
//     storage and shelf the new one,
//  4. Discard the shelf entry closest to expiry and shelf the
//     new one.
func (k *Kitchen) Place(o client.Order) {
	// Use a mutex to protect the whole placement sequence since it involves multiple steps and mutations.
	// This also simplifies the timer handling.
	k.mu.Lock()
	defer k.mu.Unlock()

	if existing, _ := k.lookup(o.ID); existing != nil {
		return
	}

	now := k.now()
	entry := &orderEntry{
		order:     o,
		remaining: time.Duration(o.Freshness) * time.Second,
		heapIdx:   -1,
	}

	if target := k.idealStore(o.Temp); target != nil && target.hasRoom() {
		k.putInTemp(entry, target, now)
		k.record(now, o, client.Place, target.name)
		return
	}

	if k.shelf.hasRoom() {
		k.putOnShelf(entry, now)
		k.record(now, o, client.Place, client.Shelf)
		return
	}

	// Relocate a non-room shelf entry back to its ideal storage
	if victim := k.shelf.findEarliestMovable(k.heater, k.cooler); victim != nil {
		k.moveShelfToIdeal(victim, now)
		k.putOnShelf(entry, now)
		k.record(now, o, client.Place, client.Shelf)
		return
	}

	// Discard earliest-expiring shelf entry to make room
	if victim := k.shelf.popEarliestExpiry(); victim != nil {
		k.stopTimer(victim)
		k.record(now, victim.order, client.Discard, client.Shelf)
	}
	k.putOnShelf(entry, now)
	k.record(now, o, client.Place, client.Shelf)
}

// Removes the named order. No-op if it has already been discarded,
// expired, or never placed.
func (k *Kitchen) Pickup(id string) {
	k.mu.Lock()
	defer k.mu.Unlock()

	e, store := k.lookup(id)
	if e == nil {
		return
	}
	k.stopTimer(e)
	k.removeFrom(e, store)
	k.record(k.now(), e.order, client.Pickup, store)
}

func (k *Kitchen) Actions() []client.Action {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]client.Action, len(k.audit.actions))
	copy(out, k.audit.actions)
	return out
}

// Test-only use.
func (k *Kitchen) ExpireForTest(id string) { k.onExpire(id) }

// -------- internals --------

func (k *Kitchen) idealStore(temp string) *tempStore {
	switch idealStorage(temp) {
	case client.Heater:
		return k.heater
	case client.Cooler:
		return k.cooler
	default:
		return nil
	}
}

func (k *Kitchen) putInTemp(e *orderEntry, target *tempStore, now time.Time) {
	e.placedAt = now
	e.storage = target.name
	e.expiresAt = expiryAt(now, e.remaining, e.decayMultiplier())
	target.add(e)
	k.armTimer(e)
}

func (k *Kitchen) putOnShelf(e *orderEntry, now time.Time) {
	e.placedAt = now
	e.storage = client.Shelf
	e.expiresAt = expiryAt(now, e.remaining, e.decayMultiplier())
	k.shelf.add(e)
	k.armTimer(e)
}

func (k *Kitchen) moveShelfToIdeal(e *orderEntry, now time.Time) {
	target := k.idealStore(e.order.Temp)
	if target == nil {
		return
	}
	elapsed := now.Sub(e.placedAt)
	consumed := time.Duration(float64(elapsed) * e.decayMultiplier())
	if consumed > e.remaining {
		consumed = e.remaining
	}
	e.remaining -= consumed
	k.shelf.remove(e)
	k.stopTimer(e)
	k.putInTemp(e, target, now)
	k.record(now, e.order, client.Move, target.name)
}

func (k *Kitchen) lookup(id string) (*orderEntry, string) {
	if e := k.heater.get(id); e != nil {
		return e, client.Heater
	}
	if e := k.cooler.get(id); e != nil {
		return e, client.Cooler
	}
	if e := k.shelf.get(id); e != nil {
		return e, client.Shelf
	}
	return nil, ""
}

func (k *Kitchen) removeFrom(e *orderEntry, storage string) {
	switch storage {
	case client.Heater:
		k.heater.remove(e)
	case client.Cooler:
		k.cooler.remove(e)
	case client.Shelf:
		k.shelf.remove(e)
	}
}

func (k *Kitchen) armTimer(e *orderEntry) {
	if !k.timers {
		return
	}
	delay := e.expiresAt.Sub(k.now())
	if delay <= 0 {
		delay = time.Microsecond
	}
	id := e.order.ID
	if e.timer != nil {
		e.timer.Stop()
	}
	e.timer = time.AfterFunc(delay, func() { k.onExpire(id) })
}

func (k *Kitchen) stopTimer(e *orderEntry) {
	if e.timer != nil {
		e.timer.Stop()
		e.timer = nil
	}
}

func (k *Kitchen) onExpire(id string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	e, store := k.lookup(id)
	if e == nil {
		return
	}
	e.timer = nil
	k.removeFrom(e, store)
	k.record(k.now(), e.order, client.Discard, store)
}

func (k *Kitchen) record(now time.Time, o client.Order, action, target string) {
	k.audit.record(now, o, action, target, snapshot{
		heater: k.heater.len(), heaterCap: k.heater.cap,
		cooler: k.cooler.len(), coolerCap: k.cooler.cap,
		shelf: k.shelf.len(), shelfCap: k.shelf.cap,
	})
}

func expiryAt(now time.Time, remaining time.Duration, decay float64) time.Time {
	return now.Add(time.Duration(float64(remaining) / decay))
}
