package runner

import (
	"context"
	"fmt"
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

func TestRun_LeadsBypassKeywordRulesAndGetTheirOwnSection(t *testing.T) {
	now := time.Date(2026, 10, 12, 8, 47, 0, 0, time.UTC)

	leadSrc := sources.Source{Name: "usaspending-awards", Fetch: func(ctx context.Context, c *http.Client, ua string) ([]model.Listing, error) {
		return []model.Listing{
			// "Catering" would fail the RFP keyword rules, but a lead
			// never runs through match.Check at all.
			{ID: "https://example.org/award/1", Kind: model.KindLead, Title: "Catering Robotics LLC", Detail: "$99,000 award from USDA: Catering automation", URL: "https://example.org/award/1"},
		}, nil
	}}
	seen := fakeSeen{seen: map[string]bool{}}

	c := cfg()
	summary := Run(context.Background(), c, seen, []sources.Source{leadSrc}, &http.Client{}, now)

	if len(summary.New) != 0 {
		t.Fatalf("expected no RFP matches, got %d", len(summary.New))
	}
	if len(summary.PostedLeads) != 1 {
		t.Fatalf("expected 1 posted lead, got %d: %+v", len(summary.PostedLeads), summary.PostedLeads)
	}
	if !strings.Contains(summary.Message, "Leads (no RFP): 1 new") {
		t.Errorf("message missing the leads section: %s", summary.Message)
	}
	if !strings.Contains(summary.Message, "Catering Robotics LLC") {
		t.Errorf("message missing the lead's title: %s", summary.Message)
	}
}

func TestRun_LeadsAreCappedPerRunAndOverflowCarriesOver(t *testing.T) {
	now := time.Now()
	leadSrc := sources.Source{Name: "eol-tech-drupal7", Fetch: func(ctx context.Context, c *http.Client, ua string) ([]model.Listing, error) {
		var out []model.Listing
		for i := 0; i < 3; i++ {
			id := fmt.Sprintf("domain-%d.org", i)
			out = append(out, model.Listing{ID: id, Kind: model.KindLead, Title: id, Detail: "Drupal 7", URL: "https://" + id})
		}
		return out, nil
	}}
	seen := fakeSeen{seen: map[string]bool{}}

	c := cfg()
	c.Leads.MaxPerRun = 2
	summary := Run(context.Background(), c, seen, []sources.Source{leadSrc}, &http.Client{}, now)

	if len(summary.NewLeads) != 3 {
		t.Fatalf("expected 3 total new leads found, got %d", len(summary.NewLeads))
	}
	if len(summary.PostedLeads) != 2 {
		t.Fatalf("expected the cap to hold posted leads to 2, got %d", len(summary.PostedLeads))
	}
	if !strings.Contains(summary.Message, "1 more new lead(s) held back") {
		t.Errorf("message should say the overflow carries over: %s", summary.Message)
	}
}

func TestRun_MeteredSourceReportsCreditsInTheMessage(t *testing.T) {
	now := time.Now()
	metered := sources.Source{Name: "daleel-madani", Fetch: func(ctx context.Context, c *http.Client, ua string) ([]model.Listing, error) {
		sources.ReportCredits(ctx, 2)
		return nil, nil
	}}
	seen := fakeSeen{seen: map[string]bool{}}

	summary := Run(context.Background(), cfg(), seen, []sources.Source{metered}, &http.Client{}, now)

	if len(summary.Counts) != 1 || summary.Counts[0].Credits != 2 {
		t.Fatalf("expected 2 credits recorded, got %+v", summary.Counts)
	}
	if !strings.Contains(summary.Message, "2 credit(s)") {
		t.Errorf("message should show credits spent: %s", summary.Message)
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
