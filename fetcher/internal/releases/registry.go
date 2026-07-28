// Package releases fetches each tracked client's GitHub releases and turns them
// into the two maps the frontend needs: the graffiti build commit (first four
// hex chars of the tag's commit SHA) -> version, and version -> publish date.
package releases

import (
	"regexp"
	"strings"

	"github.com/OffchainLabs/cl-dist/internal/codes"
)

// Repo describes one client's GitHub repository and how to read its release tags.
type Repo struct {
	Code  codes.Code
	Owner string
	Name  string

	// tagRe, when non-nil, must match a tag for it to count as a release of this
	// client, and the stored version is capture group 1. When nil, every tag is a
	// release and the version is the tag verbatim. Used for monorepos that publish
	// many per-package tags (EthereumJS).
	tagRe *regexp.Regexp
}

// version returns the version string to store for a tag, or ("", false) if the
// tag does not belong to this client.
func (r Repo) version(tag string) (string, bool) {
	if r.tagRe == nil {
		return tag, true
	}

	m := r.tagRe.FindStringSubmatch(tag)
	if m == nil {
		return "", false
	}

	return m[1], true
}

// repos is the set of tracked client repositories.
//
// Caplin (CN) is intentionally absent: it ships inside Erigon's binary, so its
// build commits and releases mirror Erigon's. Fetch aliases CN to EG afterwards.
// Nimbus (NB) is listed for completeness even though it does not yet emit the
// identification graffiti.
var repos = []Repo{
	// Execution layer.
	{Code: codes.GE, Owner: "ethereum", Name: "go-ethereum"},
	{Code: codes.NM, Owner: "NethermindEth", Name: "nethermind"},
	{Code: codes.BU, Owner: "hyperledger", Name: "besu"},
	{Code: codes.EG, Owner: "erigontech", Name: "erigon"},
	{Code: codes.RH, Owner: "paradigmxyz", Name: "reth"},
	{Code: codes.EJ, Owner: "ethereumjs", Name: "ethereumjs-monorepo", tagRe: regexp.MustCompile(`^@ethereumjs/client@(.+)$`)},
	{Code: codes.EX, Owner: "lambdaclass", Name: "ethrex"},

	// Consensus layer.
	{Code: codes.PM, Owner: "OffchainLabs", Name: "prysm"},
	{Code: codes.LH, Owner: "sigp", Name: "lighthouse"},
	{Code: codes.TK, Owner: "Consensys", Name: "teku"},
	{Code: codes.NB, Owner: "status-im", Name: "nimbus-eth2"},
	{Code: codes.LS, Owner: "ChainSafe", Name: "lodestar"},
	{Code: codes.GR, Owner: "grandinetech", Name: "grandine"},
}

// rollingTags are moving, non-versioned release tags some repos publish (nightly
// builds, branch pointers). They never correspond to a graffiti build commit.
var rollingTags = map[string]bool{
	"nightly":  true,
	"unstable": true,
	"latest":   true,
	"stable":   true,
	"head":     true,
	"master":   true,
	"main":     true,
}

func isRollingTag(tag string) bool {
	return rollingTags[strings.ToLower(tag)]
}
