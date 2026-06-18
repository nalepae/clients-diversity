// Package codes holds the registry of two-letter client identification codes
// defined by the execution-apis client identification convention, mapping each
// code to a human-readable client name.
//
// See: https://github.com/ethereum/execution-apis/blob/main/src/engine/identification.md
package codes

import "maps"

// Unknown is the bucket used for blocks whose graffiti carries no valid
// client identification segment.
const Unknown = "unknown"

// PrysmCanonical is the canonical Prysm CL code. Prysm v7.1.4 mistakenly used
// "PR"; v7.1.5+ uses "PM". We fold "PR" into "PM" at aggregation time.
const PrysmCanonical = "PM"

// EL maps execution-layer client codes to their names.
var EL = map[string]string{
	"GE": "Geth",
	"NM": "Nethermind",
	"BU": "Besu",
	"EG": "Erigon",
	"RH": "Reth",
	"EJ": "EthereumJS",
	"EX": "ethrex",
	"TE": "trin-execution",
}

// CL maps consensus-layer client codes to their names. "PR" is included as an
// alias for Prysm because v7.1.4 emitted it before the fix in v7.1.5.
var CL = map[string]string{
	"PM": "Prysm",
	"PR": "Prysm", // v7.1.4 only; folded into PM by Canonicalize.
	"LH": "Lighthouse",
	"TK": "Teku",
	"NB": "Nimbus",
	"LS": "Lodestar",
	"GR": "Grandine",
}

// IsEL reports whether code is a known execution-layer client code.
func IsEL(code string) bool {
	_, ok := EL[code]
	return ok
}

// IsCL reports whether code is a known consensus-layer client code.
func IsCL(code string) bool {
	_, ok := CL[code]
	return ok
}

// CanonicalizeCL maps a raw CL code to its canonical form. "PR" becomes "PM"
// so that both Prysm variants aggregate into a single series.
func CanonicalizeCL(code string) string {
	if code == "PR" {
		return PrysmCanonical
	}

	return code
}

// CLNames returns the canonical CL code→name map (without the "PR" alias),
// suitable for embedding in the output meta and for chart legends.
func CLNames() map[string]string {
	out := make(map[string]string, len(CL))
	for code, name := range CL {
		if code == "PR" {
			continue // folded into PM
		}
		out[code] = name
	}

	return out
}

// ELNames returns a copy of the EL code→name map for output meta / legends.
func ELNames() map[string]string {
	out := make(map[string]string, len(EL))
	maps.Copy(out, EL)
	return out
}
