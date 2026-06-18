package beacon

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGraffitiAtSlot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/eth/v1/beacon/blinded_blocks/100":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"message":{"body":{"graffiti":"0xdeadbeef"}}}}`))
		case "/eth/v1/beacon/blinded_blocks/200":
			w.WriteHeader(http.StatusNotFound) // skipped slot
		case "/eth/v1/beacon/headers/finalized":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":{"header":{"message":{"slot":"123456"}}}}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, 5*time.Second, 1)
	ctx := context.Background()

	g, found, err := c.GraffitiAtSlot(ctx, 100)
	if err != nil || !found || g != "0xdeadbeef" {
		t.Fatalf("slot 100: got (%q,%v,%v), want (0xdeadbeef,true,nil)", g, found, err)
	}

	_, found, err = c.GraffitiAtSlot(ctx, 200)
	if err != nil || found {
		t.Fatalf("slot 200 (skipped): got (found=%v, err=%v), want (false, nil)", found, err)
	}

	if _, _, err := c.GraffitiAtSlot(ctx, 999); err == nil {
		t.Fatal("slot 999: expected error on 400, got nil")
	}

	slot, err := c.FinalizedSlot(ctx)
	if err != nil || slot != 123456 {
		t.Fatalf("FinalizedSlot: got (%d,%v), want (123456,nil)", slot, err)
	}
}
