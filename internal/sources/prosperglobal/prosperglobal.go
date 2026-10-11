// Package prosperglobal adapts Mercy Corps' tenders page, published under
// its Prosper Global brand (prosperglobal.org/tenders; mercycorps.org/tenders
// redirects here).
//
// Plain server-rendered Drupal HTML: each tender is a ".c-view__row" with
// a title link, a country field, and an open/close date range. Checked
// 12 Oct 2026: 4 live tenders, deadlines read correctly against the page's
// visible dates.
package prosperglobal

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

const Name = "prosper-global"

// vars, not consts, so tests can point them at a local fixture server.
var (
	siteOrigin = "https://www.prosperglobal.org"
	pageURL    = siteOrigin + "/tenders"
)

func Fetch(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error) {
	resp, err := sources.Get(ctx, client, ua, pageURL)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &sources.BlockedError{Reason: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}

	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: parse html: %w", Name, err)
	}

	var out []model.Listing
	doc.Find(".c-view__row").Each(func(_ int, row *goquery.Selection) {
		link := row.Find("a.c-button-box__link").First()
		title := strings.TrimSpace(link.Find("h3").First().Text())
		href, _ := link.Attr("href")
		if title == "" || href == "" {
			return
		}
		fullURL := href
		if strings.HasPrefix(href, "/") {
			fullURL = siteOrigin + href
		}

		country := strings.TrimSpace(row.Find(`.c-field--name-field-countries .c-field__content`).First().Text())

		// The date field holds "<time>open</time> to <time>close</time>";
		// the closing time is the deadline. A tender with only one <time>
		// is open-ended, so Deadline stays nil.
		var deadline *time.Time
		times := row.Find(`.c-field--name-field-date time`)
		if times.Length() >= 2 {
			if dt, ok := times.Eq(times.Length() - 1).Attr("datetime"); ok {
				if t, err := time.Parse(time.RFC3339, dt); err == nil {
					deadline = &t
				}
			}
		}

		out = append(out, model.Listing{
			Source:   Name,
			ID:       fullURL,
			Title:    title,
			Buyer:    "Mercy Corps (Prosper Global)",
			Country:  country,
			Deadline: deadline,
			URL:      fullURL,
		})
	})
	return out, nil
}
