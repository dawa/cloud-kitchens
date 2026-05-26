package harness

import (
	"math/rand"
	"sync"
	"time"

	"challenge/client"
	"challenge/kitchen"
)

func Run(k *kitchen.Kitchen, orders []client.Order, rate, min, max time.Duration) []client.Action {
	var (
		wg    sync.WaitGroup
		rng   = rand.New(rand.NewSource(time.Now().UnixNano()))
		rngMu sync.Mutex
	)

	// pickup after a delay drawn uniformly from [min, max]. If max <= min, pickup after exactly min.
	pickupDelay := func() time.Duration {
		if max <= min {
			return min
		}
		rngMu.Lock() // blocks until every pickup attempt has completed
		defer rngMu.Unlock()
		return min + time.Duration(rng.Int63n(int64(max-min)))
	}

	for _, o := range orders {
		k.Place(o)
		wg.Add(1)
		go func(id string, delay time.Duration) {
			defer wg.Done()
			time.Sleep(delay)
			k.Pickup(id)
		}(o.ID, pickupDelay())
		if rate > 0 {
			time.Sleep(rate)
		}
	}
	wg.Wait()
	return k.Actions()
}
