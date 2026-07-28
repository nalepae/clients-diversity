// Package releases fetches each tracked client's GitHub releases and turns them
// into the two maps the frontend needs: the graffiti build commit (first four
// hex chars of the tag's commit SHA) -> version, and version -> publish date.
package releases

import (
	"cmp"
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

// versionRe splits a version into its numeric core and optional pre-release
// suffix: "26.2.0-RC5" -> "26.2.0" + "RC5", "v1.44.0" -> "1.44.0" + "". Names
// that are not versions at all ("push", "develop") do not match.
var versionRe = regexp.MustCompile(`^[vV]?(\d+(?:\.\d+)*)(?:[-+](.*))?$`)

// runRe splits a string into maximal digit and non-digit runs.
var runRe = regexp.MustCompile(`\d+|\D+`)

// betterVersion reports whether candidate should replace current as the version
// recorded for a build commit. Several tags routinely point at the *same*
// commit — a release and its last release candidate (Prysm v7.1.4 and
// v7.1.4-rc.3), or a stray tag and a release (Reth "push" and v1.11.3) — and
// without an explicit rule the winner is whatever GitHub happens to list last.
// Preference, in order:
//
//  1. a real version beats a non-version tag,
//  2. a final release beats its own pre-releases,
//  3. the higher version wins.
//
// The last rule also settles the rare case of two unrelated commits sharing a
// 4-hex prefix: arbitrary, but at least stable across runs.
func betterVersion(candidate, current string) bool {
	if current == "" {
		return true
	}

	aCore, aPre, aOK := splitVersion(candidate)
	bCore, bPre, bOK := splitVersion(current)

	switch {
	case aOK != bOK:
		return aOK
	case !aOK:
		return natCompare(candidate, current) > 0
	case (aPre == "") != (bPre == ""):
		return aPre == ""
	}

	if c := natCompare(aCore, bCore); c != 0 {
		return c > 0
	}

	return natCompare(aPre, bPre) > 0
}

func splitVersion(version string) (core, pre string, ok bool) {
	m := versionRe.FindStringSubmatch(version)
	if m == nil {
		return "", "", false
	}

	return m[1], m[2], true
}

// natCompare compares two version-ish strings with numeric runs compared as
// numbers, so "1.10.0" > "1.9.0" and "rc.10" > "rc.9".
func natCompare(a, b string) int {
	as, bs := runRe.FindAllString(a, -1), runRe.FindAllString(b, -1)

	for i := range min(len(as), len(bs)) {
		x, y := as[i], bs[i]

		if isDigitRun(x) && isDigitRun(y) {
			x, y = strings.TrimLeft(x, "0"), strings.TrimLeft(y, "0")
			if len(x) != len(y) {
				return cmp.Compare(len(x), len(y))
			}
		}

		if c := strings.Compare(x, y); c != 0 {
			return c
		}
	}

	return cmp.Compare(len(as), len(bs))
}

// isDigitRun reports whether s is a digit run produced by runRe.
func isDigitRun(s string) bool {
	return s != "" && s[0] >= '0' && s[0] <= '9'
}
