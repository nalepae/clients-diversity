package graffiti

import (
	"encoding/hex"
	"testing"

	"github.com/OffchainLabs/cl-dist/internal/codes"
)

// toGraffitiHex packs an ASCII string into a 32-byte, 0x-prefixed hex graffiti
// value (right-padded with NUL bytes), mirroring how nodes encode graffiti.
func toGraffitiHex(s string) string {
	var b [32]byte
	copy(b[:], s)
	return "0x" + hex.EncodeToString(b[:])
}

func TestParseText(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantEL string
		wantCL string
	}{
		{"prysm canonical", "GE117ePM5498", "GE", "PM"},
		{"nethermind prysm PR alias", "NMc07aPR1756", "NM", "PM"}, // PR folds to PM
		{"custom prefix", "ssv.network GE117ePR1756", "GE", "PM"},
		{"single-char prefix", "X GE117ePR1756", "GE", "PM"},
		{"no identification", "Everstake / Pro", codes.Unknown, codes.Unknown},
		{"empty", "", codes.Unknown, codes.Unknown},
		{"unknown codes rejected", "ZZ1234YY5678", codes.Unknown, codes.Unknown},
		{"reversed order rejected", "PM117eGE5498", codes.Unknown, codes.Unknown}, // CL first => invalid

		// Prysm's GenerateGraffiti tiers: commits truncate from 4 -> 2 -> 0 hex.
		{"reduced 2-hex commits", "GEabPMe4", "GE", "PM"},
		{"codes only", "GEPM", "GE", "PM"},
		{"codes only besu/prysm alias", "BUPR", "BU", "PM"},
		{"codes only with custom prefix", "𝕡𝟚𝕡․𝕠𝕣𝕘 BUPR", "BU", "PM"},
		{"full geth/lighthouse", "GE9566LH176c", "GE", "LH"},
		{"full erigon/caplin", "EGa53eCNa53e", "EG", "CN"},
		{"no-space concatenated full", "12345678901234567890GEabcdPMe4f6", "GE", "PM"},
		{"no-space concatenated codes only", "1234567890123456789012345678GEPM", "GE", "PM"},

		// Forms we intentionally do not identify.
		{"single code not matched", "1234567890123456789012345678901GE", codes.Unknown, codes.Unknown},
		{"segment not at end", "GE117ePM5498 trailing", codes.Unknown, codes.Unknown},
		{"code-like word rejected", "Grandine", codes.Unknown, codes.Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseText(tt.input)
			if got.EL != tt.wantEL || got.CL != tt.wantCL {
				t.Errorf("ParseText(%q) = {EL:%q CL:%q}, want {EL:%q CL:%q}",
					tt.input, got.EL, got.CL, tt.wantEL, tt.wantCL)
			}
		})
	}
}

func TestParseHex(t *testing.T) {
	got := ParseHex(toGraffitiHex("GE117ePM5498"))
	if got.EL != "GE" || got.CL != "PM" {
		t.Errorf("ParseHex = %+v, want EL=GE CL=PM", got)
	}
}

func TestDecodeHex(t *testing.T) {
	if got := DecodeHex(toGraffitiHex("GE117ePM5498")); got != "GE117ePM5498" {
		t.Errorf("DecodeHex round-trip = %q", got)
	}
	if got := DecodeHex("not-hex"); got != "" {
		t.Errorf("DecodeHex(invalid) = %q, want empty", got)
	}
}
