// Package ungm adapts the UN Global Marketplace's public notice search.
//
// Checked 12 Oct 2026: the search page's HTML has no listing rows at all,
// server side; the row area (#notices) and the result count (#noticeSearchTotal)
// are both empty placeholders that UNGM's own JavaScript fills in after the
// page loads, by a POST call this adapter would otherwise have to reproduce.
// Appending query-string filters (tried with ?Keywords=website) makes no
// difference: the server ignores them and returns the identical empty
// template either way. There is no plain-GET way to read a real result
// here, so this counts as blocked rather than something to reproduce with
// a stored session or a browser.
package ungm

import (
	"context"
	"fmt"
	"net/http"

	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

const Name = "ungm"

var url = "https://www.ungm.org/Public/Notice"

func Fetch(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error) {
	resp, err := sources.Get(ctx, client, ua, url)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &sources.BlockedError{Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	// A 200 here is still the empty search template, never real notices;
	// see the package comment. Report it as blocked every time rather
	// than parsing a page that can never carry a result.
	return nil, &sources.BlockedError{Reason: "notices load through a JavaScript POST this adapter does not reproduce; the plain GET always returns the empty search template"}
}
