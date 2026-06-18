// Package aggregate maps slots to UTC dates and tallies per-day client counts.
package aggregate

import (
	"time"

	"github.com/OffchainLabs/cl-dist/internal/codes"
	"github.com/OffchainLabs/cl-dist/internal/graffiti"
	"github.com/OffchainLabs/cl-dist/internal/store"
)

const dateLayout = "2006-01-02"

// Ethereum mainnet chain parameters used for slot↔date conversion.
const (
	MainnetGenesisTime    int64 = 1606824023 // 2020-12-01 12:00:23 UTC
	MainnetSecondsPerSlot int64 = 12
)

// Mainnet returns the Chain configured for Ethereum mainnet.
func Mainnet() Chain {
	return Chain{GenesisTime: MainnetGenesisTime, SecondsPerSlot: MainnetSecondsPerSlot}
}

// Chain holds the parameters needed to convert between slots and wall-clock time.
type Chain struct {
	GenesisTime    int64
	SecondsPerSlot int64
}

// SlotRangeForDate returns the inclusive [start, end] slot range whose slot
// start-times fall within the given UTC date.
func (c Chain) SlotRangeForDate(date time.Time) (start, end uint64) {
	dayStart := date.UTC().Truncate(24 * time.Hour)
	nextDay := dayStart.Add(24 * time.Hour)

	start = c.firstSlotAtOrAfter(dayStart.Unix())

	// Last slot strictly before next-day start.
	firstNext := c.firstSlotAtOrAfter(nextDay.Unix())
	if firstNext == 0 {
		return start, 0
	}

	return start, firstNext - 1
}

func (c Chain) firstSlotAtOrAfter(unixTime int64) uint64 {
	if unixTime <= c.GenesisTime {
		return 0
	}

	delta := unixTime - c.GenesisTime
	slot := delta / c.SecondsPerSlot
	if delta%c.SecondsPerSlot != 0 {
		slot++
	}

	return uint64(slot)
}

// DateString formats a time as the YYYY-MM-DD UTC date key.
func DateString(t time.Time) string {
	return t.UTC().Format(dateLayout)
}

// ParseDate parses a YYYY-MM-DD string as a UTC date (midnight).
func ParseDate(s string) (time.Time, error) {
	return time.ParseInLocation(dateLayout, s, time.UTC)
}

// SlotResult is the per-slot fetch outcome handed to the Tally.
type SlotResult struct {
	Found    bool
	Graffiti string
}

// Tally accumulates counts for a single day.
type Tally struct {
	total      int
	identified int
	cl         map[string]int
	el         map[string]int
}

// NewTally returns an initialized Tally with all known client buckets (and
// "unknown") pre-seeded to zero so every day record has a consistent shape.
func NewTally() *Tally {
	t := &Tally{cl: map[string]int{}, el: map[string]int{}}
	for code := range codes.CLNames() {
		t.cl[code] = 0
	}

	for code := range codes.ELNames() {
		t.el[code] = 0
	}

	t.cl[codes.Unknown] = 0
	t.el[codes.Unknown] = 0
	return t
}

// Add folds one slot result into the tally.
func (t *Tally) Add(r SlotResult) {
	if !r.Found {
		return
	}

	t.total++
	res := graffiti.ParseHex(r.Graffiti)
	t.el[res.EL]++
	t.cl[res.CL]++

	if res.EL != codes.Unknown || res.CL != codes.Unknown {
		t.identified++
	}
}

// Record finalizes the tally into a store.DayRecord for the given date.
func (t *Tally) Record(date time.Time) store.DayRecord {
	return store.DayRecord{
		Date:             DateString(date),
		TotalBlocks:      t.total,
		IdentifiedBlocks: t.identified,
		CL:               t.cl,
		EL:               t.el,
	}
}
