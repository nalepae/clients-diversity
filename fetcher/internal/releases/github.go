package releases

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"strconv"
	"time"

	"github.com/OffchainLabs/cl-dist/internal/codes"
	"github.com/OffchainLabs/cl-dist/internal/store"
)

const (
	apiBase     = "https://api.github.com"
	perPage     = 100
	buildKeyLen = 4
)

// client fetches releases from the GitHub REST API.
type client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	maxRetries int
}

// New returns a client.
func New(token string, timeout time.Duration, maxRetries int) *client {
	return &client{
		baseURL:    apiBase,
		token:      token,
		httpClient: &http.Client{Timeout: timeout},
		maxRetries: maxRetries,
	}
}

type ghRelease struct {
	TagName     string  `json:"tag_name"`
	PublishedAt *string `json:"published_at"` // RFC3339, or null for unpublished/draft
	Prerelease  bool    `json:"prerelease"`
}

// ghTag is one entry from the List-Tags endpoint. Its Commit.SHA is the commit
// the tag points to (already dereferenced).
type ghTag struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

// Fetch returns the release maps for every tracked client, or an error if any
// client's data cannot be fetched.
func (c *client) Fetch(ctx context.Context, prev *store.Releases) (*store.Releases, error) {
	out := &store.Releases{
		Builds: map[codes.Code]map[string]string{},
		Dates:  map[codes.Code]map[string]string{},
	}

	for _, repo := range repos {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		builds := copyMap(prevMap(prev, repo.Code, true))
		dates := copyMap(prevMap(prev, repo.Code, false))

		if err := c.fetchRepo(ctx, repo, builds, dates); err != nil {
			return nil, fmt.Errorf("%s (%s/%s): %w", repo.Code, repo.Owner, repo.Name, err)
		}

		out.Builds[repo.Code] = builds
		out.Dates[repo.Code] = dates
	}

	// Caplin ships inside Erigon's binary. Its releases mirror Erigon's.
	out.Builds[codes.CN] = out.Builds[codes.EG]
	out.Dates[codes.CN] = out.Dates[codes.EG]

	return out, nil
}

// fetchRepo populates builds and dates for one repo. builds/dates start as
// copies of the previous run's data.
func (c *client) fetchRepo(ctx context.Context, repo Repo, builds, dates map[string]string) error {
	needTags, err := c.fetchDates(ctx, repo, builds, dates)
	if err != nil {
		return fmt.Errorf("releases: %w", err)
	}

	// A client with no stored builds yet (first run, or one that publishes tags
	// but no GitHub releases) must scan tags regardless.
	if len(builds) == 0 {
		needTags = true
	}

	if needTags {
		if err := c.fetchBuilds(ctx, repo, builds); err != nil {
			return fmt.Errorf("tags: %w", err)
		}
	}

	return nil
}

// fetchBuilds records every tag's build commit.
//
// It uses the List-Tags endpoint, whose commit SHA is pre-dereferenced.
func (c *client) fetchBuilds(ctx context.Context, repo Repo, builds map[string]string) error {
	for page := 1; ; page++ {
		url := fmt.Sprintf("%s/repos/%s/%s/tags?per_page=%d&page=%d", c.baseURL, repo.Owner, repo.Name, perPage, page)

		var tags []ghTag
		if err := c.getJSON(ctx, url, &tags); err != nil {
			return fmt.Errorf("get json: %w", err)
		}

		for _, tag := range tags {
			if isRollingTag(tag.Name) {
				continue
			}

			version, ok := repo.version(tag.Name)
			if !ok {
				// Not this client's tag (e.g. a sibling monorepo package)
				continue
			}

			if len(tag.Commit.SHA) < buildKeyLen {
				continue
			}

			builds[tag.Commit.SHA[:buildKeyLen]] = version
		}

		if len(tags) < perPage {
			// Last (or only) page
			return nil
		}
	}
}

// fetchDates records version -> publish date from the repo's releases and
// reports whether any released version's build commit is still missing from
// builds.
func (c *client) fetchDates(ctx context.Context, repo Repo, builds, dates map[string]string) (needTags bool, err error) {
	knownVersions := make(map[string]bool, len(builds))
	for _, v := range builds {
		knownVersions[v] = true
	}

	seen := len(dates) > 0

	for page := 1; ; page++ {
		url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=%d&page=%d", c.baseURL, repo.Owner, repo.Name, perPage, page)

		var releases []ghRelease
		if err := c.getJSON(ctx, url, &releases); err != nil {
			return false, fmt.Errorf("get json: %w", err)
		}

		foundNewDate := false
		for _, rel := range releases {
			if isRollingTag(rel.TagName) {
				continue
			}

			version, ok := repo.version(rel.TagName)
			if !ok {
				continue
			}

			if !knownVersions[version] {
				needTags = true // a released version we have no build commit for
			}

			if rel.PublishedAt == nil {
				continue
			}

			t, err := time.Parse(time.RFC3339, *rel.PublishedAt)
			if err != nil {
				log.Printf("[releases] %s: skipping %q: unparseable published_at %q: %v", repo.Code, rel.TagName, *rel.PublishedAt, err)
				continue
			}

			if _, had := dates[version]; !had {
				foundNewDate = true
			}

			dates[version] = t.UTC().Format("2006-01-02")
		}

		if len(releases) < perPage {
			return needTags, nil // last (or only) page
		}

		if seen && !foundNewDate {
			// This full page added no new date. Older pages can't either, and
			// since releases are newest-first any new version was already seen.
			return needTags, nil
		}
	}
}

func (c *client) getJSON(ctx context.Context, url string, out any) error {
	var lastErr error
	for attempt := range c.maxRetries {
		wait, retryable, err := c.tryGet(ctx, url, out)
		if err == nil {
			return nil
		}

		lastErr = err
		if !retryable || attempt == c.maxRetries-1 {
			return err
		}

		// Honor a server-supplied Retry-After (GitHub sends it on secondary rate
		// limits), otherwise fall back to exponential backoff.
		if wait <= 0 {
			wait = backoff(attempt + 1)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}

	return lastErr
}

func (c *client) tryGet(ctx context.Context, url string, out any) (retryAfter time.Duration, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, false, fmt.Errorf("new request with context: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(req)
	dur := time.Since(start).Round(time.Millisecond)
	if err != nil {
		log.Printf("[releases] %s %s -> error: %v (%s)", req.Method, req.URL, err, dur)
		return 0, true, err // network error: retryable
	}
	defer resp.Body.Close()

	log.Printf("[releases] %s %s -> %d (%s)", req.Method, req.URL, resp.StatusCode, dur)

	switch {
	case resp.StatusCode == http.StatusOK:
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return 0, true, fmt.Errorf("decoding %s: %w", url, err)
		}
		return 0, false, nil

	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		// GitHub uses 403/429 for both primary and secondary rate limits.
		io.Copy(io.Discard, resp.Body)
		return retryAfterHint(resp.Header), true, fmt.Errorf("%s: status %d (rate limited)", url, resp.StatusCode)

	case resp.StatusCode >= 500:
		io.Copy(io.Discard, resp.Body)
		return 0, true, fmt.Errorf("%s: status %d", url, resp.StatusCode)

	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, false, fmt.Errorf("%s: status %d: %s", url, resp.StatusCode, string(body))
	}
}

// retryAfterHint reads how long GitHub asks us to wait: the Retry-After header
// (seconds), or the time until X-RateLimit-Reset (a Unix timestamp). It returns
// 0 when neither is usable, capped to keep a single stall bounded.
func retryAfterHint(h http.Header) time.Duration {
	const maxWait = 90 * time.Second

	if v := h.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return min(time.Duration(secs)*time.Second, maxWait)
		}
	}

	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if reset, err := strconv.ParseInt(v, 10, 64); err == nil {
			if d := time.Until(time.Unix(reset, 0)); d > 0 {
				return min(d, maxWait)
			}
		}
	}

	return 0
}

func backoff(attempt int) time.Duration {
	return min(time.Duration(attempt)*500*time.Millisecond, 4*time.Second)
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	maps.Copy(out, m)

	return out
}

// prevMap returns the previous builds (builds=true) or dates (builds=false) map
// for a code, or nil when there is no prior data.
func prevMap(prev *store.Releases, code codes.Code, builds bool) map[string]string {
	if prev == nil {
		return nil
	}

	if builds {
		return prev.Builds[code]
	}

	return prev.Dates[code]
}
