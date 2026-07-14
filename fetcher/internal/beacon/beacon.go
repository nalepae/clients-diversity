// Package beacon is a minimal client for the standard Beacon API, used to fetch
// the graffiti of a block at a given slot and the latest finalized slot.
package beacon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/OffchainLabs/cl-dist/internal/aggregate"
	"github.com/OffchainLabs/cl-dist/internal/codes"
	"github.com/OffchainLabs/cl-dist/internal/graffiti"
)

// blockTimeUTC returns the wall-clock start time (UTC) of a mainnet slot.
func blockTimeUTC(slot uint64) time.Time {
	return time.Unix(aggregate.MainnetGenesisTime+int64(slot)*aggregate.MainnetSecondsPerSlot, 0).UTC()
}

// Client talks to a single beacon node REST endpoint.
type Client struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
}

// New returns a new Client.
func New(baseURL string, timeout time.Duration, maxRetries int) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
		maxRetries: maxRetries,
	}
}

func (c *Client) do(req *http.Request) (*http.Response, time.Duration, error) {
	start := time.Now()
	resp, err := c.httpClient.Do(req)
	return resp, time.Since(start).Round(time.Millisecond), err
}

func logReq(req *http.Request, status int, dur time.Duration, extra string) {
	if extra != "" {
		extra = " " + extra
	}

	log.Printf("[beacon] %s %s -> %d (%s)%s", req.Method, req.URL, status, dur, extra)
}

func logReqErr(req *http.Request, dur time.Duration, err error) {
	log.Printf("[beacon] %s %s -> error: %v (%s)", req.Method, req.URL, err, dur)
}

type blockResponse struct {
	Data struct {
		Message struct {
			Body struct {
				Graffiti string `json:"graffiti"`
			} `json:"body"`
		} `json:"message"`
	} `json:"data"`
}

// GraffitiAtSlot returns the graffiti hex string for the block at slot.
func (c *Client) GraffitiAtSlot(ctx context.Context, slot uint64) (graffitiHex string, found bool, err error) {
	url := c.baseURL + "/eth/v1/beacon/blinded_blocks/" + strconv.FormatUint(slot, 10)

	var lastErr error
	for attempt := range c.maxRetries {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", false, ctx.Err()

			case <-time.After(backoff(attempt)):
			}
		}

		g, ok, retryable, e := c.tryGraffiti(ctx, url, slot)
		if e == nil {
			return g, ok, nil
		}

		lastErr = e
		if !retryable {
			return "", false, e
		}
	}

	return "", false, fmt.Errorf("slot %d: %w", slot, lastErr)
}

func (c *Client) tryGraffiti(ctx context.Context, url string, slot uint64) (graffitiHex string, found, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", false, false, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, dur, err := c.do(req)
	if err != nil {
		logReqErr(req, dur, err)
		return "", false, true, fmt.Errorf("performing request: %w", err)
	}
	defer resp.Body.Close()

	extra := "time=" + blockTimeUTC(slot).Format(time.RFC3339)
	defer func() { logReq(req, resp.StatusCode, dur, extra) }()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		// No block proposed at this slot (skipped/orphaned).
		io.Copy(io.Discard, resp.Body)
		return "", false, false, nil

	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		io.Copy(io.Discard, resp.Body)
		return "", false, true, fmt.Errorf("beacon returned status %d", resp.StatusCode)

	case resp.StatusCode != http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", false, false, fmt.Errorf("beacon returned status %d: %s", resp.StatusCode, string(body))
	}

	var br blockResponse
	if err := json.NewDecoder(resp.Body).Decode(&br); err != nil {
		return "", false, true, fmt.Errorf("decoding block response: %w", err)
	}

	graffitiHex = br.Data.Message.Body.Graffiti
	text := graffiti.DecodeHex(graffitiHex)
	res := graffiti.ParseText(text)
	extra += fmt.Sprintf(" graffiti=%q EL=%s CL=%s", text, res.EL, res.CL)
	if res.EL != codes.Unknown || res.CL != codes.Unknown {
		extra += " ✅"
	}

	return graffitiHex, true, false, nil
}

type headerResponse struct {
	Data struct {
		Header struct {
			Message struct {
				Slot string `json:"slot"`
			} `json:"message"`
		} `json:"header"`
	} `json:"data"`
}

// FinalizedSlot returns the slot of the latest finalized block, used to decide
// whether a day's blocks are safe to ingest. Transient errors are retried.
func (c *Client) FinalizedSlot(ctx context.Context) (uint64, error) {
	url := c.baseURL + "/eth/v1/beacon/headers/finalized"

	var lastErr error
	for attempt := range c.maxRetries {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()

			case <-time.After(backoff(attempt)):
			}
		}

		slot, retryable, e := c.tryFinalized(ctx, url)
		if e == nil {
			return slot, nil
		}

		lastErr = e
		if !retryable {
			return 0, e
		}
	}

	return 0, fmt.Errorf("finalized header: %w", lastErr)
}

func (c *Client) tryFinalized(ctx context.Context, url string) (slot uint64, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, false, err
	}
	req.Header.Set("Accept", "application/json")

	resp, dur, err := c.do(req)
	if err != nil {
		logReqErr(req, dur, err)
		return 0, true, err // network error: retryable
	}
	defer resp.Body.Close()
	defer func() { logReq(req, resp.StatusCode, dur, "") }()

	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		io.Copy(io.Discard, resp.Body)
		return 0, true, fmt.Errorf("beacon returned status %d", resp.StatusCode)

	case resp.StatusCode != http.StatusOK:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, false, fmt.Errorf("beacon returned status %d: %s", resp.StatusCode, string(body))
	}

	var hr headerResponse
	if err := json.NewDecoder(resp.Body).Decode(&hr); err != nil {
		return 0, true, fmt.Errorf("decoding header response: %w", err)
	}

	s, err := strconv.ParseUint(hr.Data.Header.Message.Slot, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("parsing finalized slot %q: %w", hr.Data.Header.Message.Slot, err)
	}

	return s, false, nil
}

func backoff(attempt int) time.Duration {
	return min(time.Duration(attempt)*250*time.Millisecond, 2*time.Second)
}
