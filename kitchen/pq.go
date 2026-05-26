package kitchen

import "container/heap"

// expiryHeap is a min-heap of *orderEntry ordered by expiresAt.
// Each entry stores it's current heap index so Remove and Fix run in O(log n)
type expiryHeap []*orderEntry

func (h expiryHeap) Len() int { return len(h) }

func (h expiryHeap) Less(i, j int) bool {
	return h[i].expiresAt.Before(h[j].expiresAt)
}

func (h expiryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIdx = i
	h[j].heapIdx = j
}

func (h *expiryHeap) Push(x any) {
	e := x.(*orderEntry)
	e.heapIdx = len(*h)
	*h = append(*h, e)
}

func (h *expiryHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	old[n-1] = nil
	e.heapIdx = -1
	*h = old[:n-1]
	return e
}

func (h *expiryHeap) add(e *orderEntry)    { heap.Push(h, e) }
func (h *expiryHeap) fix(e *orderEntry)    { heap.Fix(h, e.heapIdx) }
func (h *expiryHeap) remove(e *orderEntry) { heap.Remove(h, e.heapIdx) }

func (h *expiryHeap) popEarliest() *orderEntry {
	if len(*h) == 0 {
		return nil
	}
	return heap.Pop(h).(*orderEntry)
}
