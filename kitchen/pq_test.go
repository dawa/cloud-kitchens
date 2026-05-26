package kitchen

import (
	"testing"
	"time"

	"challenge/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mkEntry(id string, expiresAt time.Time) *orderEntry {
	return &orderEntry{
		order:     client.Order{ID: id},
		expiresAt: expiresAt,
		heapIdx:   -1,
	}
}

func TestIndexedHeap_PopReturnsEarliestExpiry(t *testing.T) {
	base := time.Now()
	a := mkEntry("a", base.Add(3*time.Second))
	b := mkEntry("b", base.Add(1*time.Second))
	c := mkEntry("c", base.Add(2*time.Second))

	var h expiryHeap
	h.add(a)
	h.add(b)
	h.add(c)

	require.Equal(t, "b", h.popEarliest().order.ID)
	require.Equal(t, "c", h.popEarliest().order.ID)
	require.Equal(t, "a", h.popEarliest().order.ID)
	require.Nil(t, h.popEarliest())
}

func TestIndexedHeap_RemoveArbitrary(t *testing.T) {
	base := time.Now()
	a := mkEntry("a", base.Add(1*time.Second))
	b := mkEntry("b", base.Add(2*time.Second))
	c := mkEntry("c", base.Add(3*time.Second))

	var h expiryHeap
	h.add(a)
	h.add(b)
	h.add(c)

	h.remove(b)
	assert.Equal(t, -1, b.heapIdx, "removed entry should have heapIdx reset")
	require.Equal(t, "a", h.popEarliest().order.ID)
	require.Equal(t, "c", h.popEarliest().order.ID)
}

func TestIndexedHeap_FixAfterKeyChange(t *testing.T) {
	base := time.Now()
	a := mkEntry("a", base.Add(1*time.Second))
	b := mkEntry("b", base.Add(2*time.Second))
	c := mkEntry("c", base.Add(3*time.Second))

	var h expiryHeap
	h.add(a)
	h.add(b)
	h.add(c)

	// Move a far into the future so it should sink to the bottom.
	a.expiresAt = base.Add(10 * time.Second)
	h.fix(a)

	require.Equal(t, "b", h.popEarliest().order.ID)
	require.Equal(t, "c", h.popEarliest().order.ID)
	require.Equal(t, "a", h.popEarliest().order.ID)
}

func TestIndexedHeap_IndicesStayConsistent(t *testing.T) {
	base := time.Now()
	entries := make([]*orderEntry, 16)
	var h expiryHeap
	for i := range entries {
		entries[i] = mkEntry(string(rune('a'+i)), base.Add(time.Duration(15-i)*time.Second))
		h.add(entries[i])
	}
	// Verify every entry knows its real index in the heap array.
	for i, e := range h {
		require.Equal(t, i, e.heapIdx, "heap index mismatch at position %d", i)
	}
	// Removing an arbitrary entry preserves the invariant.
	h.remove(entries[5])
	for i, e := range h {
		require.Equal(t, i, e.heapIdx, "heap index mismatch after remove at %d", i)
	}
}
