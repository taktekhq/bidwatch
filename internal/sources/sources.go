// Package sources defines the shared contract every adapter implements and
// the error type an adapter returns when the source blocks the request
// instead of answering it.
package sources

import (
	"context"
	"fmt"
	"net/http"

	"github.com/taktekhq/bidwatch/internal/model"
)

// BlockedError means the source answered but refused the request: a
// Cloudflare or similar challenge page, a 403, or a page that only renders
// through JavaScript with no plain-GET fallback. An adapter that sees this
// returns BlockedError and does nothing else; it never retries with a
// different user agent, a cookie jar, or any other way around the block.
type BlockedError struct {
	Reason string
}

func (e *BlockedError) Error() string { return e.Reason }

// Fetcher is the contract every source adapter implements. ua is the
// honest, bot-naming user agent to send; it is never a browser fingerprint.
type Fetcher func(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error)

// Source pairs a name with its adapter for the run loop and config.
type Source struct {
	Name  string
	Fetch Fetcher
}

// Get issues a single GET with the given honest user agent and a short
// timeout already set on client. It never retries and never follows a
// redirect to a different host's login page silently; http.Client's
// default redirect policy (follow, same semantics as curl -L) is fine for
// the plain-HTML sources here.
func Get(ctx context.Context, client *http.Client, ua, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml,application/rss+xml;q=0.9,*/*;q=0.8")
	return client.Do(req)
}

// CloudflareChallenge reports whether a response is a Cloudflare bot
// challenge rather than the real page, the same signature the kbase
// web-scraping notes use: a cf-mitigated header, or the challenge script
// path in a small body.
func CloudflareChallenge(resp *http.Response, body []byte) bool {
	if resp.Header.Get("Cf-Mitigated") != "" {
		return true
	}
	if resp.StatusCode == http.StatusForbidden && resp.Header.Get("Server") == "cloudflare" {
		return true
	}
	return false
}

// creditsKey is the context key a metered adapter (one that spends a paid
// API's credits, like Firecrawl) reports its spend on, without changing
// the Fetcher signature every adapter implements.
type creditsKey struct{}

// WithCredits returns a context carrying a credits sink for one source's
// fetch. The runner calls this once per source so a per-run count of what
// got spent can be shown in the Slack message and runs.log, even though
// most adapters never spend anything and never call ReportCredits.
func WithCredits(ctx context.Context, sink *int) context.Context {
	return context.WithValue(ctx, creditsKey{}, sink)
}

// ReportCredits adds n to ctx's credits sink, if Run gave it one. An
// adapter with nothing metered to report never calls this.
func ReportCredits(ctx context.Context, n int) {
	if sink, ok := ctx.Value(creditsKey{}).(*int); ok {
		*sink += n
	}
}
