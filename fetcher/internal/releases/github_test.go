package releases

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// fakeGitHub serves canned List-Tags (/tags) and List-Releases (/releases)
// responses on page 1; later pages come back empty.
type fakeGitHub struct {
	tags     string // page-1 /tags JSON
	releases string // page-1 /releases JSON
}

func (f fakeGitHub) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstPage := r.URL.Query().Get("page") == "" || r.URL.Query().Get("page") == "1"
		switch {
		case strings.HasSuffix(r.URL.Path, "/tags"):
			if firstPage {
				io.WriteString(w, f.tags)
			} else {
				io.WriteString(w, "[]")
			}
		case strings.HasSuffix(r.URL.Path, "/releases"):
			if firstPage {
				io.WriteString(w, f.releases)
			} else {
				io.WriteString(w, "[]")
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func testClient(srv *httptest.Server) *client {
	return &client{baseURL: srv.URL, httpClient: srv.Client(), maxRetries: 2}
}

func TestFetchRepo_BuildsFromTagsDatesFromReleases(t *testing.T) {
	fake := fakeGitHub{
		tags: `[
			{"name":"1.39.2","commit":{"sha":"6568910591e4618dc49d54285b6213c3753d7243"}},
			{"name":"1.40.0","commit":{"sha":"abcd1234000000000000000000000000000000ab"}},
			{"name":"nightly","commit":{"sha":"ffffffff00000000000000000000000000000000"}},
			{"name":"1.10.0","commit":{"sha":"deadbeef00000000000000000000000000000000"}}
		]`,
		releases: `[
			{"tag_name":"1.39.2","published_at":"2026-06-01T00:00:00Z"},
			{"tag_name":"1.40.0","published_at":null},
			{"tag_name":"nightly","published_at":"2026-06-02T00:00:00Z"},
			{"tag_name":"1.10.0","published_at":"2025-06-01T00:00:00Z"}
		]`,
	}
	srv := fake.server(t)
	defer srv.Close()

	repo := Repo{Code: "NM", Owner: "NethermindEth", Name: "nethermind"}
	builds, dates := map[string]string{}, map[string]string{}
	if err := testClient(srv).fetchRepo(context.Background(), repo, builds, dates); err != nil {
		t.Fatalf("fetchRepo: %v", err)
	}

	// Every non-rolling tag is a build, keyed by the first 4 hex of its commit;
	// "nightly" is dropped.
	wantBuilds := map[string]string{"6568": "1.39.2", "abcd": "1.40.0", "dead": "1.10.0"}
	if !reflect.DeepEqual(builds, wantBuilds) {
		t.Errorf("builds = %v, want %v", builds, wantBuilds)
	}

	// Dates come from published_at; 1.40.0 (null) has none, nightly is dropped.
	wantDates := map[string]string{"1.39.2": "2026-06-01", "1.10.0": "2025-06-01"}
	if !reflect.DeepEqual(dates, wantDates) {
		t.Errorf("dates = %v, want %v", dates, wantDates)
	}
}

func TestFetchBuilds_MonorepoTagFilter(t *testing.T) {
	fake := fakeGitHub{
		tags: `[
			{"name":"@ethereumjs/client@0.10.5","commit":{"sha":"9e46ffff00000000000000000000000000000000"}},
			{"name":"@ethereumjs/vm@10.1.2","commit":{"sha":"1111222200000000000000000000000000000000"}}
		]`,
	}
	srv := fake.server(t)
	defer srv.Close()

	repo := Repo{
		Code:  "EJ",
		Owner: "ethereumjs",
		Name:  "ethereumjs-monorepo",
		tagRe: regexp.MustCompile(`^@ethereumjs/client@(.+)$`),
	}
	builds := map[string]string{}
	if err := testClient(srv).fetchBuilds(context.Background(), repo, builds); err != nil {
		t.Fatalf("fetchBuilds: %v", err)
	}

	// The client tag is kept (version stripped of prefix); the vm tag is ignored.
	wantBuilds := map[string]string{"9e46": "0.10.5"}
	if !reflect.DeepEqual(builds, wantBuilds) {
		t.Errorf("builds = %v, want %v", builds, wantBuilds)
	}
}

// pagedReleasesServer serves a full first page of 100 releases (v0..v99) then a
// single older release "v999" on page 2, recording which pages were requested.
func pagedReleasesServer(t *testing.T, pagesHit *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		page := r.URL.Query().Get("page")
		*pagesHit = append(*pagesHit, page)
		if page == "1" {
			var b strings.Builder
			b.WriteString("[")
			for i := range 100 {
				if i > 0 {
					b.WriteString(",")
				}
				fmt.Fprintf(&b, `{"tag_name":"v%d","published_at":"2026-01-01T00:00:00Z"}`, i)
			}
			b.WriteString("]")
			io.WriteString(w, b.String())
			return
		}
		io.WriteString(w, `[{"tag_name":"v999","published_at":"2020-01-01T00:00:00Z"}]`)
	}))
}

func TestFetchDates_IncrementalStopsWhenAllKnown(t *testing.T) {
	var pages []string
	srv := pagedReleasesServer(t, &pages)
	defer srv.Close()

	// builds and dates already hold every version on page 1, so nothing is new and
	// paging stops after the first page.
	builds, dates := map[string]string{}, map[string]string{}
	for i := range 100 {
		builds[fmt.Sprintf("%04x", i)] = fmt.Sprintf("v%d", i)
		dates[fmt.Sprintf("v%d", i)] = "2026-01-01"
	}

	repo := Repo{Code: "NM", Owner: "o", Name: "n"}
	needTags, err := testClient(srv).fetchDates(context.Background(), repo, builds, dates)
	if err != nil {
		t.Fatalf("fetchDates: %v", err)
	}
	if needTags {
		t.Error("needTags = true, want false (every released version is already recorded)")
	}
	if !reflect.DeepEqual(pages, []string{"1"}) {
		t.Errorf("pages requested = %v, want [1] (must stop once a page adds nothing new)", pages)
	}
}

func TestFetchDates_ColdRunReadsAllPagesAndNeedsTags(t *testing.T) {
	var pages []string
	srv := pagedReleasesServer(t, &pages)
	defer srv.Close()

	repo := Repo{Code: "NM", Owner: "o", Name: "n"}
	dates := map[string]string{}
	needTags, err := testClient(srv).fetchDates(context.Background(), repo, map[string]string{}, dates)
	if err != nil {
		t.Fatalf("fetchDates: %v", err)
	}
	if !needTags {
		t.Error("needTags = false, want true (nothing recorded yet)")
	}
	if len(dates) != 101 {
		t.Errorf("dates = %d entries, want 101 (full history)", len(dates))
	}
	if !reflect.DeepEqual(pages, []string{"1", "2"}) {
		t.Errorf("pages requested = %v, want [1 2] (cold run reads to the end)", pages)
	}
}

// recordingServer serves canned single-page /tags and /releases and records
// which endpoint each request hit ("tags" or "releases").
func recordingServer(t *testing.T, tags, releases string, hits *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstPage := r.URL.Query().Get("page") == "1"
		switch {
		case strings.HasSuffix(r.URL.Path, "/tags"):
			*hits = append(*hits, "tags")
			if firstPage {
				io.WriteString(w, tags)
			} else {
				io.WriteString(w, "[]")
			}
		case strings.HasSuffix(r.URL.Path, "/releases"):
			*hits = append(*hits, "releases")
			if firstPage {
				io.WriteString(w, releases)
			} else {
				io.WriteString(w, "[]")
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestFetchRepo_SkipsTagsWhenNothingNew(t *testing.T) {
	var hits []string
	// The only release is one we already have a build for, so tags must be skipped.
	srv := recordingServer(t, `[]`, `[{"tag_name":"1.39.2","published_at":"2026-06-01T00:00:00Z"}]`, &hits)
	defer srv.Close()

	repo := Repo{Code: "NM", Owner: "NethermindEth", Name: "nethermind"}
	builds := map[string]string{"6568": "1.39.2"}
	dates := map[string]string{"1.39.2": "2026-06-01"}
	if err := testClient(srv).fetchRepo(context.Background(), repo, builds, dates); err != nil {
		t.Fatalf("fetchRepo: %v", err)
	}

	for _, h := range hits {
		if h == "tags" {
			t.Fatalf("tags endpoint was queried; requests = %v, want releases only", hits)
		}
	}
	if !reflect.DeepEqual(builds, map[string]string{"6568": "1.39.2"}) {
		t.Errorf("builds mutated = %v", builds)
	}
}

func TestFetchRepo_ScansTagsWhenNewRelease(t *testing.T) {
	var hits []string
	tags := `[
		{"name":"1.40.0","commit":{"sha":"abcd1234000000000000000000000000000000ab"}},
		{"name":"1.39.2","commit":{"sha":"6568910591e4618dc49d54285b6213c3753d7243"}}
	]`
	releases := `[
		{"tag_name":"1.40.0","published_at":"2026-07-01T00:00:00Z"},
		{"tag_name":"1.39.2","published_at":"2026-06-01T00:00:00Z"}
	]`
	srv := recordingServer(t, tags, releases, &hits)
	defer srv.Close()

	repo := Repo{Code: "NM", Owner: "NethermindEth", Name: "nethermind"}
	builds := map[string]string{"6568": "1.39.2"} // 1.40.0 is new
	dates := map[string]string{"1.39.2": "2026-06-01"}
	if err := testClient(srv).fetchRepo(context.Background(), repo, builds, dates); err != nil {
		t.Fatalf("fetchRepo: %v", err)
	}

	if builds["abcd"] != "1.40.0" {
		t.Errorf("builds[abcd] = %q, want 1.40.0 (tags should have been scanned)", builds["abcd"])
	}
	sawTags := false
	for _, h := range hits {
		if h == "tags" {
			sawTags = true
		}
	}
	if !sawTags {
		t.Errorf("tags endpoint was not queried; requests = %v", hits)
	}
}

func TestFetchDates_NullDateKnownViaBuildsIsNotNew(t *testing.T) {
	// A release with a null published_at never enters the dates map, but if its
	// build commit is already known it must not be treated as new (which would
	// re-scan tags every run).
	fake := fakeGitHub{releases: `[{"tag_name":"v7.1.8","published_at":null}]`}
	srv := fake.server(t)
	defer srv.Close()

	repo := Repo{Code: "PM", Owner: "OffchainLabs", Name: "prysm"}
	builds := map[string]string{"8db5": "v7.1.8"} // commit already captured
	dates := map[string]string{"v7.1.7": "2026-07-13"}
	needTags, err := testClient(srv).fetchDates(context.Background(), repo, builds, dates)
	if err != nil {
		t.Fatalf("fetchDates: %v", err)
	}
	if needTags {
		t.Error("needTags = true, want false (null-date version is known via builds)")
	}
}

func TestFetchRepo_ErrorPropagates(t *testing.T) {
	// A hard 404 (non-retryable) must surface as an error, not be swallowed —
	// release ingestion is all-or-nothing. The releases endpoint is hit first.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"message":"Not Found"}`)
	}))
	defer srv.Close()

	repo := Repo{Code: "NM", Owner: "o", Name: "n"}
	err := testClient(srv).fetchRepo(context.Background(), repo, map[string]string{}, map[string]string{})
	if err == nil {
		t.Fatal("fetchRepo returned nil, want an error on 404")
	}
	if !strings.Contains(err.Error(), "releases") {
		t.Errorf("error = %v, want it to mention the failing releases request", err)
	}
}

func TestFetchBuilds_SharedCommitPrefersRelease(t *testing.T) {
	// Real shape of the GitHub /tags response: newest first, and a release sits on
	// the same commit as its last candidate (Prysm v7.1.4) or as a stray tag
	// (Reth "push" / v1.11.3). The release must win either way.
	fake := fakeGitHub{
		tags: `[
			{"name":"v7.1.4","commit":{"sha":"1756000000000000000000000000000000000000"}},
			{"name":"v7.1.4-rc.3","commit":{"sha":"1756000000000000000000000000000000000000"}},
			{"name":"v7.1.4-rc.2","commit":{"sha":"1ab5000000000000000000000000000000000000"}},
			{"name":"push","commit":{"sha":"d632000000000000000000000000000000000000"}},
			{"name":"v1.11.3","commit":{"sha":"d632000000000000000000000000000000000000"}},
			{"name":"26.2.0-RC4","commit":{"sha":"fade000000000000000000000000000000000000"}},
			{"name":"26.2.0-RC5","commit":{"sha":"e5e9000000000000000000000000000000000000"}},
			{"name":"26.2.0","commit":{"sha":"e5e9000000000000000000000000000000000000"}}
		]`,
	}
	srv := fake.server(t)
	defer srv.Close()

	repo := Repo{Code: "PM", Owner: "o", Name: "n"}
	// A previous run recorded the candidate: the rescan must correct it.
	builds := map[string]string{"1756": "v7.1.4-rc.3"}
	if err := testClient(srv).fetchBuilds(context.Background(), repo, builds); err != nil {
		t.Fatalf("fetchBuilds: %v", err)
	}

	wantBuilds := map[string]string{
		"1756": "v7.1.4",
		"1ab5": "v7.1.4-rc.2", // candidate alone on its commit: kept
		"d632": "v1.11.3",
		"fade": "26.2.0-RC4",
		"e5e9": "26.2.0",
	}
	if !reflect.DeepEqual(builds, wantBuilds) {
		t.Errorf("builds = %v, want %v", builds, wantBuilds)
	}
}

func TestBetterVersion(t *testing.T) {
	for _, tc := range []struct {
		candidate, current string
		want               bool
		why                string
	}{
		{"v7.1.4", "", true, "first tag on a commit always wins"},
		{"v7.1.4", "v7.1.4-rc.3", true, "release beats its candidate"},
		{"v7.1.4-rc.3", "v7.1.4", false, "candidate never beats its release"},
		{"26.2.0", "26.2.0-RC5", true, "uppercase RC suffix"},
		{"v0.35.0", "v0.35.0-beta.0", true, "beta suffix"},
		{"v1.11.3", "push", true, "version beats a non-version tag"},
		{"push", "v1.11.3", false, "non-version tag never beats a version"},
		{"v1.28.1-rc.1", "v1.28.1-rc.0", true, "later candidate wins"},
		{"v1.28.1-rc.0", "v1.28.1-rc.1", false, "earlier candidate loses"},
		{"26.2.0-RC10", "26.2.0-RC9", true, "candidate numbers compare numerically"},
		{"v1.23.0", "v1.9.2", true, "unrelated commits colliding: higher version"},
		{"v1.9.2", "v1.23.0", false, "and stably so, whichever order they arrive"},
		{"0.10.5", "0.10.5", false, "identical is not better"},
	} {
		if got := betterVersion(tc.candidate, tc.current); got != tc.want {
			t.Errorf("betterVersion(%q, %q) = %v, want %v (%s)", tc.candidate, tc.current, got, tc.want, tc.why)
		}
	}
}

func TestIsRollingTag(t *testing.T) {
	for _, tag := range []string{"nightly", "Nightly", "unstable", "latest"} {
		if !isRollingTag(tag) {
			t.Errorf("isRollingTag(%q) = false, want true", tag)
		}
	}
	for _, tag := range []string{"v1.2.3", "1.39.2", "v1.44.0-rc.1"} {
		if isRollingTag(tag) {
			t.Errorf("isRollingTag(%q) = true, want false", tag)
		}
	}
}
