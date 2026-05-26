package kitchen

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"challenge/client"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- helpers ----------

func newTestKitchen() *Kitchen {
	return New(WithOutput(io.Discard), WithoutTimers())
}

func order(id, name, temp string, freshness int) client.Order {
	return client.Order{ID: id, Name: name, Temp: temp, Price: 1, Freshness: freshness}
}

func actionTargets(actions []client.Action, id string) []string {
	var out []string
	for _, a := range actions {
		if a.ID == id {
			out = append(out, a.Action+":"+a.Target)
		}
	}
	return out
}

// ---------- placement tests ----------

func TestPlace_RoutesToIdealStorage(t *testing.T) {
	k := newTestKitchen()
	k.Place(order("h1", "Burger", "hot", 60))
	k.Place(order("c1", "Ice", "cold", 60))
	k.Place(order("r1", "Bread", "room", 60))

	assert.Equal(t, 1, k.heater.len())
	assert.Equal(t, 1, k.cooler.len())
	assert.Equal(t, 1, k.shelf.len())
}

func TestPlace_OverflowsToShelf(t *testing.T) {
	k := newTestKitchen()
	for i := 0; i < HeaterCap; i++ {
		k.Place(order(fmt.Sprintf("h%d", i), "Burger", "hot", 60))
	}
	// One more HOT order — heater is full, should land on shelf.
	k.Place(order("hX", "Burger", "hot", 60))
	assert.Equal(t, HeaterCap, k.heater.len())
	assert.Equal(t, 1, k.shelf.len())

	targets := actionTargets(k.Actions(), "hX")
	require.Equal(t, []string{"place:shelf"}, targets)
}

func TestPlace_MovesShelfOrderBackToIdeal(t *testing.T) {
	k := newTestKitchen()
	// Fill the heater.
	for i := 0; i < HeaterCap; i++ {
		k.Place(order(fmt.Sprintf("h%d", i), "Burger", "hot", 60))
	}
	// Spill one HOT to the shelf.
	k.Place(order("hShelf", "Burger", "hot", 60))
	// Fill the shelf with ROOM orders.
	for i := 0; i < ShelfCap-1; i++ {
		k.Place(order(fmt.Sprintf("r%d", i), "Bread", "room", 60))
	}
	require.Equal(t, ShelfCap, k.shelf.len())
	require.Equal(t, HeaterCap, k.heater.len())

	// Free one heater slot so a relocation becomes possible.
	k.Pickup("h0")
	require.Equal(t, HeaterCap-1, k.heater.len())

	// A new ROOM order — ideal (shelf) full, no other place to put it
	// without moving "hShelf" back to the heater.
	k.Place(order("rNew", "Bread", "room", 60))

	assert.Equal(t, HeaterCap, k.heater.len(), "moved entry should refill heater")
	assert.Equal(t, ShelfCap, k.shelf.len(), "new order should occupy shelf")
	assert.Contains(t, actionTargets(k.Actions(), "hShelf"), "move:heater")
	assert.Contains(t, actionTargets(k.Actions(), "rNew"), "place:shelf")
}

func TestPlace_DiscardsEarliestExpiringWhenStuck(t *testing.T) {
	k := newTestKitchen()
	// Fill heater so HOT spills to shelf.
	for i := 0; i < HeaterCap; i++ {
		k.Place(order(fmt.Sprintf("h%d", i), "Burger", "hot", 60))
	}
	// Shelf: 12 ROOM orders. The earliest-expiring is the one with the
	// shortest freshness — make r0 the victim.
	k.Place(order("r0", "Bread", "room", 1)) // expires almost immediately
	for i := 1; i < ShelfCap; i++ {
		k.Place(order(fmt.Sprintf("r%d", i), "Bread", "room", 600))
	}
	require.Equal(t, ShelfCap, k.shelf.len())
	require.Equal(t, HeaterCap, k.heater.len())

	// Step 3 (move) cannot help — no shelf entry is HOT/COLD.
	// Step 4 (discard earliest expiring) must drop r0.
	k.Place(order("rNew", "Bread", "room", 60))

	// r0 was discarded; rNew shelved.
	_, store := k.lookup("r0")
	assert.Empty(t, store, "r0 should be discarded")
	_, store = k.lookup("rNew")
	assert.Equal(t, client.Shelf, store, "rNew should be on shelf")
	assert.Contains(t, actionTargets(k.Actions(), "r0"), "discard:shelf")
}

func TestPlace_DuplicateIDIsIdempotent(t *testing.T) {
	k := newTestKitchen()
	k.Place(order("dup", "Burger", "hot", 60))
	first, store := k.lookup("dup")
	require.NotNil(t, first)
	require.Equal(t, client.Heater, store)
	beforeActions := len(k.Actions())

	// Second Place with the same ID — even with different fields —
	// must not evict, re-insert, or record another action.
	k.Place(order("dup", "Burger", "hot", 1))

	again, _ := k.lookup("dup")
	assert.Same(t, first, again, "duplicate Place must not replace the existing entry")
	assert.Equal(t, 1, k.heater.len(), "duplicate Place must not add a second entry")
	assert.Equal(t, beforeActions, len(k.Actions()), "duplicate Place must not record an action")
}

func TestPlace_DuplicateDoesNotEvictUnderShelfPressure(t *testing.T) {
	k := newTestKitchen()
	// Fill the shelf with ROOM orders; one of them is our duplicate target.
	k.Place(order("dup", "Bread", "room", 600))
	for i := 1; i < ShelfCap; i++ {
		k.Place(order(fmt.Sprintf("r%d", i), "Bread", "room", 600))
	}
	require.Equal(t, ShelfCap, k.shelf.len())
	beforeActions := len(k.Actions())

	// Re-Placing "dup" must short-circuit; nothing should be discarded
	// to make room for it.
	k.Place(order("dup", "Bread", "room", 600))

	assert.Equal(t, ShelfCap, k.shelf.len(), "shelf count must be unchanged")
	assert.Equal(t, beforeActions, len(k.Actions()), "no new action should be recorded")
}

// ---------- pickup tests ----------

func TestPickup_RemovesWithoutAffectingOthers(t *testing.T) {
	k := newTestKitchen()
	k.Place(order("a", "X", "hot", 60))
	k.Place(order("b", "Y", "hot", 60))
	k.Place(order("c", "Z", "hot", 60))

	// Capture map of remaining entries before pickup.
	require.Equal(t, 3, k.heater.len())
	k.Pickup("b")
	assert.Equal(t, 2, k.heater.len())
	// a and c still present, untouched.
	a, _ := k.lookup("a")
	c, _ := k.lookup("c")
	require.NotNil(t, a)
	require.NotNil(t, c)
	assert.Equal(t, "X", a.order.Name)
	assert.Equal(t, "Z", c.order.Name)
}

func TestPickup_MissingIsNoop(t *testing.T) {
	k := newTestKitchen()
	k.Place(order("a", "X", "hot", 60))
	before := len(k.Actions())
	k.Pickup("nonexistent")
	after := len(k.Actions())
	assert.Equal(t, before, after, "pickup of missing order must not record an action")
}

// ---------- expiry / decay tests ----------

func TestExpiry_AutoDiscardsRemovesOrder(t *testing.T) {
	k := newTestKitchen()
	k.Place(order("a", "Burger", "hot", 60))
	k.ExpireForTest("a")

	_, store := k.lookup("a")
	assert.Empty(t, store, "expired entry should be gone")
	assert.Contains(t, actionTargets(k.Actions(), "a"), "discard:heater")
}

func TestDecay_NonRoomOnShelfHalvesLifetime(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	k := New(WithOutput(io.Discard), WithoutTimers(), WithClock(func() time.Time { return now }))

	// Fill heater to force HOT order onto shelf.
	for i := 0; i < HeaterCap; i++ {
		k.Place(order(fmt.Sprintf("h%d", i), "Burger", "hot", 60))
	}
	k.Place(order("hot-on-shelf", "Burger", "hot", 60))

	e, store := k.lookup("hot-on-shelf")
	require.NotNil(t, e)
	require.Equal(t, client.Shelf, store)
	// HOT on shelf: 60s of freshness consumed at 2x → expires in 30s.
	assert.Equal(t, now.Add(30*time.Second), e.expiresAt)
}

func TestFreshness_PreservedAcrossMove(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	clock := t0
	k := New(WithOutput(io.Discard), WithoutTimers(), WithClock(func() time.Time { return clock }))

	// Fill heater so HOT spills to shelf.
	for i := 0; i < HeaterCap; i++ {
		k.Place(order(fmt.Sprintf("h%d", i), "Burger", "hot", 60))
	}
	k.Place(order("hShelf", "Burger", "hot", 60))
	// Fill remaining shelf with ROOM so the move-back path triggers later.
	for i := 0; i < ShelfCap-1; i++ {
		k.Place(order(fmt.Sprintf("r%d", i), "Bread", "room", 600))
	}

	// Advance 10s; hShelf consumes 20s at 2x → remaining 40s.
	clock = t0.Add(10 * time.Second)
	// Pick up a heater entry so the move-back will succeed.
	k.Pickup("h0")
	// Trigger move-back: ROOM new arrival, shelf full, ideal (shelf) full.
	k.Place(order("rTrigger", "Bread", "room", 600))

	e, store := k.lookup("hShelf")
	require.NotNil(t, e)
	require.Equal(t, client.Heater, store, "hShelf should now be in heater")
	// remaining = 40s, decay 1x → expiresAt = clock + 40s.
	assert.Equal(t, clock.Add(40*time.Second), e.expiresAt)
}

// ---------- audit log shape ----------

func TestActions_AreTimestampMonotonic(t *testing.T) {
	k := newTestKitchen()
	for i := 0; i < 30; i++ {
		k.Place(order(fmt.Sprintf("o%d", i), "X", "hot", 60))
	}
	actions := k.Actions()
	for i := 1; i < len(actions); i++ {
		assert.GreaterOrEqual(t, actions[i].Timestamp, actions[i-1].Timestamp,
			"action %d timestamp regressed", i)
	}
}

// ---------- concurrency stress ----------

func TestConcurrency_NoLossOrOrphans(t *testing.T) {
	k := New(WithOutput(io.Discard), WithoutTimers())

	const N = 500
	var wg sync.WaitGroup
	var picked int32

	for i := 0; i < N; i++ {
		o := order(fmt.Sprintf("o%d", i), "X", []string{"hot", "cold", "room"}[i%3], 600)
		wg.Add(1)
		go func(o client.Order) {
			defer wg.Done()
			k.Place(o)
			// 50% pickup ratio.
			if o.Price%2 == 1 {
				return
			}
			k.Pickup(o.ID)
			atomic.AddInt32(&picked, 1)
		}(o)
	}
	wg.Wait()

	actions := k.Actions()
	// At minimum: N place actions in the log.
	placeCount := 0
	pickupCount := 0
	for _, a := range actions {
		switch a.Action {
		case client.Place:
			placeCount++
		case client.Pickup:
			pickupCount++
		}
	}
	assert.Equal(t, N, placeCount, "every order should produce a place action")
	assert.Equal(t, int(picked), pickupCount, "every successful pickup should be logged")
}
