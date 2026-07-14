// Package graffiti parses the client-identification segment.
package graffiti

import (
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/OffchainLabs/cl-dist/internal/codes"
)

var identRe = regexp.MustCompile(`([A-Z]{2})([0-9a-f]{0,8})([A-Z]{2})([0-9a-f]{0,8})$`)

// Result holds the parsed client codes.
type Result struct {
	EL codes.Code
	CL codes.Code
}

// DecodeHex converts a 0x-prefixed (or bare) hex graffiti string into its ASCII
// text form.
func DecodeHex(graffitiHex string) string {
	s := strings.TrimPrefix(strings.TrimSpace(graffitiHex), "0x")
	raw, err := hex.DecodeString(s)
	if err != nil {
		return ""
	}

	raw = bytesTrimRightNul(raw)
	return strings.TrimSpace(string(raw))
}

func bytesTrimRightNul(b []byte) []byte {
	i := len(b)
	for i > 0 && b[i-1] == 0x00 {
		i--
	}

	return b[:i]
}

// ParseText extracts the client codes from an already-decoded graffiti string.
func ParseText(text string) Result {
	if m := identRe.FindStringSubmatch(text); m != nil {
		el, cl := codes.Code(m[1]), codes.Code(m[3])
		if codes.IsEL(el) && codes.IsCL(cl) {
			return Result{EL: el, CL: codes.CanonicalizeCL(cl)}
		}
	}

	return Result{EL: codes.Unknown, CL: codes.Unknown}
}

// ParseHex decodes a hex graffiti value and parses its client codes.
func ParseHex(graffitiHex string) Result {
	return ParseText(DecodeHex(graffitiHex))
}
