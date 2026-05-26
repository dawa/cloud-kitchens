package kitchen

import "challenge/client"

// tempStore is a fixed-capacity bucket keyed by order ID.
type tempStore struct {
	name string
	cap  int
	byID map[string]*orderEntry
}

func newTempStore(name string, cap int) *tempStore {
	return &tempStore{name: name, cap: cap, byID: make(map[string]*orderEntry, cap)}
}

func (s *tempStore) hasRoom() bool             { return len(s.byID) < s.cap }
func (s *tempStore) len() int                  { return len(s.byID) }
func (s *tempStore) get(id string) *orderEntry { return s.byID[id] }

func (s *tempStore) add(e *orderEntry) {
	s.byID[e.order.ID] = e
	e.storage = s.name
}

func (s *tempStore) remove(e *orderEntry) {
	delete(s.byID, e.order.ID)
}

// shelf holds at most cap entries plus an indexed min-heap on expiresAt so
// the discard path finds the next-to-expire item in O(log n).
type shelf struct {
	cap  int
	byID map[string]*orderEntry
	heap expiryHeap
}

func newShelf(cap int) *shelf {
	return &shelf{cap: cap, byID: make(map[string]*orderEntry, cap)}
}

func (s *shelf) hasRoom() bool             { return len(s.byID) < s.cap }
func (s *shelf) len() int                  { return len(s.byID) }
func (s *shelf) get(id string) *orderEntry { return s.byID[id] }

func (s *shelf) add(e *orderEntry) {
	s.byID[e.order.ID] = e
	e.storage = client.Shelf
	s.heap.add(e)
}

func (s *shelf) remove(e *orderEntry) {
	delete(s.byID, e.order.ID)
	s.heap.remove(e)
}

func (s *shelf) popEarliestExpiry() *orderEntry {
	e := s.heap.popEarliest()
	if e != nil {
		delete(s.byID, e.order.ID)
	}
	return e
}

// Pick the one with the earliest expiresAt to savethe most-threatened order.
// If the shelf capacity is very large this becomes inefficient
// TODO: Perhaps use a separate heap for each temp
func (s *shelf) findEarliestMovable(heater, cooler *tempStore) *orderEntry {
	var best *orderEntry
	for _, e := range s.byID {
		var target *tempStore
		switch idealStorage(e.order.Temp) {
		case client.Heater:
			target = heater
		case client.Cooler:
			target = cooler
		default:
			continue
		}
		if !target.hasRoom() {
			continue
		}
		if best == nil || e.expiresAt.Before(best.expiresAt) {
			best = e
		}
	}
	return best
}
