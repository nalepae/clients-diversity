// Package store defines the on-disk JSON data model and provides atomic
// load/save of the single data.json file that serves as both the datastore and
// the frontend's data source.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/OffchainLabs/cl-dist/internal/codes"
)

// Meta holds run metadata for the frontend. Code→name legends are a display
// concern and live in the frontend, not here.
type Meta struct {
	GeneratedAt       string `json:"generatedAt"`
	StartDate         string `json:"startDate"`
	LastCompletedDate string `json:"lastCompletedDate"`
	GenesisTime       int64  `json:"genesisTime"`
	SecondsPerSlot    int64  `json:"secondsPerSlot"`
}

// DayRecord holds aggregated per-day counts for one UTC date.
type DayRecord struct {
	Date             string                        `json:"date"` // YYYY-MM-DD (UTC)
	TotalBlocks      int                           `json:"totalBlocks"`
	IdentifiedBlocks int                           `json:"identifiedBlocks"`
	CL               map[codes.Code]int            `json:"cl"`
	EL               map[codes.Code]int            `json:"el"`
	CLReleases       map[codes.Code]map[string]int `json:"clReleases"`
	ELReleases       map[codes.Code]map[string]int `json:"elReleases"`
}

// DataFile is the root JSON document.
type DataFile struct {
	Meta Meta        `json:"meta"`
	Days []DayRecord `json:"days"`
}

// Load reads the data file at path. A missing file yields an empty DataFile
// (not an error) so the first run starts from scratch.
func Load(path string) (*DataFile, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &DataFile{}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var df DataFile
	if err := json.Unmarshal(b, &df); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &df, nil
}

// Save writes the data file atomically.
func Save(path string, df *DataFile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	b, err := json.MarshalIndent(df, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling data: %w", err)
	}
	b = append(b, '\n')

	tmp, err := os.CreateTemp(dir, ".data-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op if rename succeeded

	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("syncing temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renaming temp file into place: %w", err)
	}

	return nil
}

// AppendDay appends a completed day record and advances LastCompletedDate.
func (df *DataFile) AppendDay(rec DayRecord) {
	df.Days = append(df.Days, rec)
	df.Meta.LastCompletedDate = rec.Date
}
