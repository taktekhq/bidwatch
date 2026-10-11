// Package daleelmadani adapts Daleel Madani's "Calls for Proposals" board
// (Lebanese NGOs and INGOs).
//
// Checked 12 Oct 2026: a plain GET with an honest User-Agent gets a
// Cloudflare challenge page (HTTP 403, Cf-Mitigated: challenge), not the
// listing. Per the rule never to route around a block, this adapter does
// the one GET, recognizes the challenge, and reports it as blocked. It does
// not try a browser, a different UA, or cookies.
package daleelmadani

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

const Name = "daleel-madani"

var url = "https://daleel-madani.org/calls-for-proposal"

func Fetch(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error) {
	resp, err := sources.Get(ctx, client, ua, url)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", Name, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if sources.CloudflareChallenge(resp, body) {
		return nil, &sources.BlockedError{Reason: "Cloudflare challenge on the calls-for-proposal page"}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &sources.BlockedError{Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	// A real 200 has never happened against this adapter (checked 12 Oct
	// 2026); the block lifting would need a parser written against the
	// real page first, not a guess here.
	return nil, fmt.Errorf("%s: got HTTP 200 but no parser exists yet for a real page; check by hand before trusting this", Name)
}
