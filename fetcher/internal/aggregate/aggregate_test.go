package aggregate

import (
	"testing"
	"time"

	"github.com/OffchainLabs/cl-dist/internal/codes"
)

func TestSlotRangeForDate(t *testing.T) {
	// Mainnet params.
	c := Chain{GenesisTime: 1606824023, SecondsPerSlot: 12}
	date, _ := ParseDate("2026-05-22")

	start, end := c.SlotRangeForDate(date)

	if start >= end {
		t.Fatalf("start %d >= end %d", start, end)
	}
	// A UTC day has exactly 7200 slots (86400/12); boundary rounding keeps it
	// at 7199 or 7200 slots inclusive.
	count := end - start + 1
	if count != 7200 {
		t.Errorf("slot count = %d, want 7200", count)
	}
	// Every slot in range must map back into the date.
	startTime := time.Unix(c.GenesisTime+int64(start)*c.SecondsPerSlot, 0).UTC()
	if DateString(startTime) != "2026-05-22" {
		t.Errorf("start slot %d maps to %s, want 2026-05-22", start, DateString(startTime))
	}
	endTime := time.Unix(c.GenesisTime+int64(end)*c.SecondsPerSlot, 0).UTC()
	if DateString(endTime) != "2026-05-22" {
		t.Errorf("end slot %d maps to %s, want 2026-05-22", end, DateString(endTime))
	}
}

func TestTally(t *testing.T) {
	tally := NewTally()
	tally.Add(SlotResult{Found: false})                                    // skipped slot
	tally.Add(SlotResult{Found: true, Graffiti: hexOf("GE117ePM5498")})    // Geth + Prysm
	tally.Add(SlotResult{Found: true, Graffiti: hexOf("Everstake / Pro")}) // unknown

	rec := tally.Record(time.Now())
	if rec.TotalBlocks != 2 {
		t.Errorf("TotalBlocks = %d, want 2", rec.TotalBlocks)
	}
	if rec.IdentifiedBlocks != 1 {
		t.Errorf("IdentifiedBlocks = %d, want 1", rec.IdentifiedBlocks)
	}
	if rec.EL["GE"] != 1 || rec.CL["PM"] != 1 {
		t.Errorf("expected GE=1 PM=1, got EL=%v CL=%v", rec.EL, rec.CL)
	}
	if rec.EL[codes.Unknown] != 1 || rec.CL[codes.Unknown] != 1 {
		t.Errorf("expected unknown=1 for both layers, got EL=%d CL=%d", rec.EL[codes.Unknown], rec.CL[codes.Unknown])
	}
}

func TestTallyReleases(t *testing.T) {
	tally := NewTally()
	// Same Prysm release, three commit-truncation tiers (4-hex, 2-hex, dropped),
	// plus a distinct Geth build on the last one.
	tally.Add(SlotResult{Found: true, Graffiti: hexOf("GE117ePM5498")}) // GE 117e, PM 5498
	tally.Add(SlotResult{Found: true, Graffiti: hexOf("GEabPM54")})     // GE ab,   PM 54
	tally.Add(SlotResult{Found: true, Graffiti: hexOf("GEPM")})         // GE "",   PM ""
	tally.Add(SlotResult{Found: true, Graffiti: hexOf("Everstake")})    // unknown

	rec := tally.Record(time.Now())

	wantCL := map[string]int{"5498": 1, "54": 1, "": 1}
	if got := rec.CLReleases["PM"]; !mapsEqual(got, wantCL) {
		t.Errorf("CLReleases[PM] = %v, want %v", got, wantCL)
	}

	wantEL := map[string]int{"117e": 1, "ab": 1, "": 1}
	if got := rec.ELReleases["GE"]; !mapsEqual(got, wantEL) {
		t.Errorf("ELReleases[GE] = %v, want %v", got, wantEL)
	}

	// Invariant: a client's release counts sum to its total.
	if sum := sumValues(rec.CLReleases["PM"]); sum != rec.CL["PM"] {
		t.Errorf("CLReleases[PM] sums to %d, want CL[PM]=%d", sum, rec.CL["PM"])
	}
	if sum := sumValues(rec.ELReleases["GE"]); sum != rec.EL["GE"] {
		t.Errorf("ELReleases[GE] sums to %d, want EL[GE]=%d", sum, rec.EL["GE"])
	}

	// Unknown blocks carry no commit, so never appear in the release maps.
	if _, ok := rec.CLReleases[codes.Unknown]; ok {
		t.Errorf("CLReleases must not contain the Unknown bucket")
	}
}

func mapsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func sumValues(m map[string]int) int {
	sum := 0
	for _, v := range m {
		sum += v
	}
	return sum
}

// hexOf packs an ASCII string into a 32-byte 0x-prefixed hex graffiti value.
func hexOf(s string) string {
	var b [32]byte
	copy(b[:], s)
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 2+len(b)*2)
	out[0], out[1] = '0', 'x'
	for i, c := range b {
		out[2+i*2] = hexdigits[c>>4]
		out[2+i*2+1] = hexdigits[c&0x0f]
	}
	return string(out)
}
