package main

import (
	"testing"
	"time"
)

func TestNextRunUTC(t *testing.T) {
	mustParse := func(s string) time.Time {
		ts, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return ts
	}

	tests := []struct {
		name string
		now  string
		want string
	}{
		{"before 01:00 same day", "2026-06-17T00:30:00Z", "2026-06-17T01:00:00Z"},
		{"exactly 01:00 rolls to next day", "2026-06-17T01:00:00Z", "2026-06-18T01:00:00Z"},
		{"after 01:00 next day", "2026-06-17T09:00:00Z", "2026-06-18T01:00:00Z"},
		{"just before midnight", "2026-06-17T23:59:59Z", "2026-06-18T01:00:00Z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextRunUTC(mustParse(tt.now))
			if !got.Equal(mustParse(tt.want)) {
				t.Errorf("nextRunUTC(%s) = %s, want %s", tt.now, got.Format(time.RFC3339), tt.want)
			}
		})
	}
}
