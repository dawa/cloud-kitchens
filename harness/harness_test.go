package harness

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"challenge/client"
	"challenge/kitchen"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_PlacesAndPicksUpEveryOrder(t *testing.T) {
	var buf bytes.Buffer
	var bufMu sync.Mutex
	k := kitchen.New(
		kitchen.WithOutput(&lockedWriter{w: &buf, mu: &bufMu}),
	)

	orders := []client.Order{
		{ID: "a", Name: "Burger", Temp: "hot", Price: 10, Freshness: 30},
		{ID: "b", Name: "Salad", Temp: "cold", Price: 8, Freshness: 30},
		{ID: "c", Name: "Bread", Temp: "room", Price: 5, Freshness: 30},
	}

	// Tight timings keep the test fast.
	actions := Run(k, orders, 5*time.Millisecond, 10*time.Millisecond, 20*time.Millisecond)

	places, pickups := 0, 0
	for _, a := range actions {
		switch a.Action {
		case client.Place:
			places++
		case client.Pickup:
			pickups++
		}
	}
	require.Equal(t, len(orders), places)
	require.Equal(t, len(orders), pickups, "every order should be picked up within the test window")
	assert.NotEmpty(t, buf.String(), "console printer should have emitted output")
}

func TestRun_LargeStreamStaysConsistent(t *testing.T) {
	k := kitchen.New(kitchen.WithOutput(discard{}))
	orders := make([]client.Order, 50)
	for i := range orders {
		orders[i] = client.Order{
			ID:        fmt.Sprintf("o%d", i),
			Name:      "X",
			Temp:      []string{"hot", "cold", "room"}[i%3],
			Freshness: 5,
		}
	}
	actions := Run(k, orders, 1*time.Millisecond, 5*time.Millisecond, 15*time.Millisecond)

	// Every order must appear in the log at least once.
	seen := make(map[string]bool, len(orders))
	for _, a := range actions {
		if a.Action == client.Place {
			seen[a.ID] = true
		}
	}
	for _, o := range orders {
		assert.True(t, seen[o.ID], "missing place for order %s", o.ID)
	}
}

// lockedWriter serializes writes — the production printer is already called
// under the kitchen mutex, but a test-local writer with mutex makes intent
// explicit when sharing with goroutines.
type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (lw *lockedWriter) Write(p []byte) (int, error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return lw.w.Write(p)
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
