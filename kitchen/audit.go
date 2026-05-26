package kitchen

import (
	"fmt"
	"io"
	"strings"
	"time"

	"challenge/client"
)

// Captures occupancy of each storage at the moment an action runs.
type snapshot struct {
	heater, cooler, shelf          int
	heaterCap, coolerCap, shelfCap int
}

// All operations must be called with the Kitchen's mutex held, so timestamps stay monotonic per-kitchen.
type audit struct {
	out     io.Writer
	actions []client.Action
	started time.Time
}

func newAudit(out io.Writer, started time.Time) *audit {
	return &audit{out: out, started: started}
}

func (a *audit) record(now time.Time, o client.Order, action, target string, snap snapshot) {
	a.actions = append(a.actions, client.Action{
		Timestamp: now.UnixMicro(),
		ID:        o.ID,
		Action:    action,
		Target:    target,
	})
	if a.out == nil {
		return
	}
	fmt.Fprintf(a.out,
		"[t=%9.3fs] %-7s order=%-8s %-22s %-5s -> %-7s  [H:%2d/%d C:%2d/%d S:%2d/%d]\n",
		now.Sub(a.started).Seconds(),
		strings.ToUpper(action),
		truncID(o.ID),
		fmt.Sprintf("%q", o.Name),
		strings.ToUpper(o.Temp),
		target,
		snap.heater, snap.heaterCap,
		snap.cooler, snap.coolerCap,
		snap.shelf, snap.shelfCap,
	)
}

func (a *audit) Actions() []client.Action {
	return a.actions
}

func truncID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
