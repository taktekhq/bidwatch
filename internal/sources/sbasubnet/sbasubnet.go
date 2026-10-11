// Package sbasubnet adapts the SBA's SUBNet subcontracting board: federal
// prime contractors post subcontracting opportunities here under their
// SBA small-business subcontracting plans.
//
// Plain server-rendered Drupal Views HTML, no login. Checked 12 Oct 2026:
// 10 open postings on the one page this adapter reads (no keyword filter
// applied at the source; bidwatch's own match rules do that), every one
// of them construction, trades or facilities work, which is the normal
// shape of this board, not a sign anything is broken.
package sbasubnet

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

const Name = "sba-subnet"

// vars, not consts, so tests can point them at a local fixture server.
var (
	baseURL = "https://legacy.sba.gov"
	pageURL = baseURL + "/federal-contracting/contracting-guide/prime-subcontracting/subcontracting-opportunities"
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
	doc.Find("table.usa-table tbody tr").Each(func(_ int, tr *goquery.Selection) {
		titleCell := tr.Find(`td[headers="view-body-table-column"]`).First()
		link := titleCell.Find("span.subnet_title a").First()
		title := strings.TrimSpace(link.Text())
		href, _ := link.Attr("href")
		buyer := strings.TrimSpace(titleCell.Find("span.subnet_business_name").First().Text())
		if title == "" || href == "" {
			return
		}
		fullURL := href
		if strings.HasPrefix(href, "/") {
			fullURL = baseURL + href
		}

		closing := strings.TrimSpace(tr.Find(`td[headers="view-field-subnet-closing-timestamp-table-column"]`).First().Text())
		var deadline *time.Time
		if closing != "" {
			if t, err := time.Parse("01/02/2006", closing); err == nil {
				deadline = &t
			}
		}

		out = append(out, model.Listing{
			Source:   Name,
			ID:       fullURL,
			Title:    title,
			Buyer:    buyer,
			Deadline: deadline,
			URL:      fullURL,
		})
	})
	return out, nil
}
