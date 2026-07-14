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

// Chain holds the parameters needed to convert between slots and wall-clock time.
type Chain struct {
	GenesisTime    int64
	SecondsPerSlot int64
}

// Mainnet returns the Chain configured for Ethereum mainnet.
func Mainnet() Chain {
	return Chain{GenesisTime: MainnetGenesisTime, SecondsPerSlot: MainnetSecondsPerSlot}
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

// SlotStartTime returns the wall-clock UTC start time of a slot.
func (c Chain) SlotStartTime(slot uint64) time.Time {
	return time.Unix(c.GenesisTime+int64(slot)*c.SecondsPerSlot, 0).UTC()
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
	cl         map[codes.Code]int
	el         map[codes.Code]int
	clReleases map[codes.Code]map[string]int
	elReleases map[codes.Code]map[string]int
}

// NewTally returns an initialized Tally with all known client buckets.
func NewTally() *Tally {
	tally := &Tally{
		cl:         map[codes.Code]int{},
		el:         map[codes.Code]int{},
		clReleases: map[codes.Code]map[string]int{},
		elReleases: map[codes.Code]map[string]int{},
	}

	for code := range codes.CL {
		canonical := codes.CanonicalizeCL(code)
		tally.cl[canonical] = 0
		tally.clReleases[canonical] = map[string]int{}
	}

	for code := range codes.EL {
		tally.el[code] = 0
		tally.elReleases[code] = map[string]int{}
	}

	tally.cl[codes.Unknown] = 0
	tally.el[codes.Unknown] = 0

	return tally
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

	// The Unknown bucket has no commit, so only identified clients contribute to
	// the per-release breakdown.
	if res.EL != codes.Unknown {
		t.elReleases[res.EL][res.ELCommit]++
	}

	if res.CL != codes.Unknown {
		t.clReleases[res.CL][res.CLCommit]++
	}

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
		CLReleases:       t.clReleases,
		ELReleases:       t.elReleases,
	}
}
