package savethechildren

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/taktekhq/bidwatch/internal/sources"
)

func TestFetch_CloudflareChallengeIsBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cf-Mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("<html>just a moment...</html>"))
	}))
	defer srv.Close()

	orig := url
	url = srv.URL
	defer func() { url = orig }()

	_, err := Fetch(context.Background(), srv.Client(), "test-ua")
	if err == nil {
		t.Fatal("expected a blocked error")
	}
	if _, ok := err.(*sources.BlockedError); !ok {
		t.Fatalf("expected *sources.BlockedError, got %T: %v", err, err)
	}
}
