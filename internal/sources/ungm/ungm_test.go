package ungm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/taktekhq/bidwatch/internal/sources"
)

func TestFetch_EmptyTemplateIsBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<div id="noticesTotal"></div><div id="notices"></div>`))
	}))
	defer srv.Close()

	orig := url
	url = srv.URL
	defer func() { url = orig }()

	_, err := Fetch(context.Background(), srv.Client(), "test-ua")
	if err == nil {
		t.Fatal("expected a blocked error even on HTTP 200, since the template never carries real notices")
	}
	if _, ok := err.(*sources.BlockedError); !ok {
		t.Fatalf("expected *sources.BlockedError, got %T: %v", err, err)
	}
}
