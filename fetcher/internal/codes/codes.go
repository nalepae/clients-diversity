// Package codes holds the registry of two-letter client identification codes
// defined by the execution-apis client identification convention.
//
// See: https://github.com/ethereum/execution-apis/blob/main/src/engine/identification.md
package codes

// Code is a client identification code: a known two-letter client code, the
// "PR" Prysm alias, or the Unknown sentinel.
type Code string

// Unknown is the bucket used for blocks whose graffiti carries no valid
// client identification segment.
const Unknown Code = "unknown"

// Execution-layer client codes.
const (
	BU Code = "BU" // Besu
	EG Code = "EG" // Erigon
	EJ Code = "EJ" // EthereumJS
	EX Code = "EX" // Ethrex
	GE Code = "GE" // Geth
	NM Code = "NM" // Nethermind
	RH Code = "RH" // Reth
	TE Code = "TE" // Trin-execution
)

// Consensus-layer client codes.
const (
	CN Code = "CN" // Caplin
	GR Code = "GR" // Grandine
	LH Code = "LH" // Lighthouse
	LS Code = "LS" // Lodestar
	NB Code = "NB" // Nimbus
	PM Code = "PM" // Prysm V7.1.5+
	PR Code = "PR" // Prysm V7.1.4
	TK Code = "TK" // Teku
)

// EL is the set of known execution-layer client codes.
var EL = map[Code]bool{
	BU: true,
	EG: true,
	EJ: true,
	EX: true,
	GE: true,
	NM: true,
	RH: true,
	TE: true,
}

// CL is the set of known consensus-layer client codes.
var CL = map[Code]bool{
	CN: true,
	GR: true,
	LH: true,
	LS: true,
	NB: true,
	PM: true,
	PR: true,
	TK: true,
}

// IsEL reports whether code is a known execution-layer client code.
func IsEL(code Code) bool {
	return EL[code]
}

// IsCL reports whether code is a known consensus-layer client code.
func IsCL(code Code) bool {
	return CL[code]
}

// CanonicalizeCL maps a raw CL code to its canonical form. PR becomes PM so
// that both Prysm variants aggregate into a single series.
func CanonicalizeCL(code Code) Code {
	if code == PR {
		return PM
	}

	return code
}
