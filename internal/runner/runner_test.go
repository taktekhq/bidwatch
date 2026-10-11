package runner

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/taktekhq/bidwatch/internal/config"
	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

type fakeSeen struct{ seen map[string]bool }

func (f fakeSeen) IsNew(id string) bool { return !f.seen[id] }

func cfg() *config.Config {
	c := &config.Config{}
	c.UserAgent = "test-ua"
	c.DelayBetweenSources = 0
	c.Match.Keywords = []string{"website", "software"}
	c.Match.Excludes = []string{"construction"}
	return c
}

func TestRun_ReportsNewMatchesAndSourceCounts(t *testing.T) {
	now := time.Date(2026, 10, 12, 8, 47, 0, 0, time.UTC)
	future := now.AddDate(0, 0, 10)

	ok := sources.Source{Name: "ok-source", Fetch: func(ctx context.Context, c *http.Client, ua string) ([]model.Listing, error) {
		return []model.Listing{
			{ID: "https://example.org/new", Title: "Website Redesign", Buyer: "Acme", Deadline: &future, URL: "https://example.org/new"},
			{ID: "https://example.org/already-seen", Title: "Software Build", Buyer: "Acme", URL: "https://example.org/already-seen"},
			{ID: "https://example.org/excluded", Title: "Website and construction work", URL: "https://example.org/excluded"},
			{ID: "https://example.org/no-keyword", Title: "Catering services", URL: "https://example.org/no-keyword"},
		}, nil
	}}
	blocked := sources.Source{Name: "blocked-source", Fetch: func(ctx context.Context, c *http.Client, ua string) ([]model.Listing, error) {
		return nil, &sources.BlockedError{Reason: "Cloudflare challenge"}
	}}

	seen := fakeSeen{seen: map[string]bool{"https://example.org/already-seen": true}}

	summary := Run(context.Background(), cfg(), seen, []sources.Source{ok, blocked}, &http.Client{}, now)

	if len(summary.New) != 1 {
		t.Fatalf("expected 1 new match, got %d: %+v", len(summary.New), summary.New)
	}
	if summary.New[0].ID != "https://example.org/new" {
		t.Errorf("new match ID = %q", summary.New[0].ID)
	}

	if len(summary.Counts) != 2 {
		t.Fatalf("expected 2 source counts, got %d", len(summary.Counts))
	}
	okCount := summary.Counts[0]
	if okCount.Fetched != 4 || okCount.Matched != 2 || okCount.New != 1 {
		t.Errorf("ok-source count = %+v", okCount)
	}
	blockedCount := summary.Counts[1]
	if blockedCount.Blocked != "Cloudflare challenge" {
		t.Errorf("blocked-source count = %+v", blockedCount)
	}

	if !strings.Contains(summary.Message, "1 new match") {
		t.Errorf("message missing new-match count: %s", summary.Message)
	}
	if !strings.Contains(summary.Message, "Website Redesign") {
		t.Errorf("message missing the new listing's title: %s", summary.Message)
	}
	if strings.Contains(summary.Message, "Software Build") {
		t.Errorf("message should not repeat an already-seen listing: %s", summary.Message)
	}
	if !strings.Contains(summary.Message, "blocked-source blocked (Cloudflare challenge)") {
		t.Errorf("message missing the blocked source's reason: %s", summary.Message)
	}
	if strings.Contains(summary.Message, "—") {
		t.Errorf("message must never contain an em dash: %s", summary.Message)
	}
}

func TestRun_NoNewMatchesStillNamesEverySource(t *testing.T) {
	now := time.Now()
	empty := sources.Source{Name: "empty-source", Fetch: func(ctx context.Context, c *http.Client, ua string) ([]model.Listing, error) {
		return nil, nil
	}}
	seen := fakeSeen{seen: map[string]bool{}}

	summary := Run(context.Background(), cfg(), seen, []sources.Source{empty}, &http.Client{}, now)

	if len(summary.New) != 0 {
		t.Fatalf("expected no new matches, got %d", len(summary.New))
	}
	if !strings.Contains(summary.Message, "no new matches") {
		t.Errorf("message = %q", summary.Message)
	}
	if !strings.Contains(summary.Message, "empty-source") {
		t.Errorf("a dead or quiet source must still be named: %s", summary.Message)
	}
}
