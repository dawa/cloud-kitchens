package kitchen

import (
	"strings"
	"time"

	"challenge/client"
)

// Temperature values used by the order feed.
const (
	TempHot  = "hot"
	TempCold = "cold"
	TempRoom = "room"
)

// orderEntry tracks one order through its life in a Kitchen.
type orderEntry struct {
	order     client.Order
	storage   string        // heater | cooler | shelf
	placedAt  time.Time     // moment order placed in storage
	remaining time.Duration // freshness remaining when it entered storage
	expiresAt time.Time     // placedAt + remaining/decayMultiplier
	heapIdx   int           // index in the shelf heap; -1 if not on shelf
	timer     *time.Timer   // fires onExpire(order.ID); nil when not armed
}

// decayMultiplier is the freshness consumption rate in the entry's storage.
// Non-room food on the shelf decays at 2x; everything else at 1x.
func (e *orderEntry) decayMultiplier() float64 {
	if e.storage == client.Shelf && !strings.EqualFold(e.order.Temp, TempRoom) {
		return 2.0
	}
	return 1.0
}

// idealStorage returns the storage where the given temperature does not
// suffer accelerated decay.
func idealStorage(temp string) string {
	switch strings.ToLower(temp) {
	case TempHot:
		return client.Heater
	case TempCold:
		return client.Cooler
	default:
		return client.Shelf
	}
}
