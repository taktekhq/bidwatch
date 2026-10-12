// Package usaspending adapts USAspending.gov's public award-search API
// (api.usaspending.gov, no auth, no key) to find small, recent federal
// software/web/IT contract awards that look like real build work, as
// leads to approach the winning company as a subcontractor. These are
// never RFPs: usaspending shows what already got awarded, so there's
// nothing open to bid on by the time it shows up here.
//
// Checked 13 Oct 2026, after the first real run surfaced mostly noise
// (license renewals, named-brand hardware/software purchases, medical
// equipment, one recipient appearing five times): the query now asks
// USAspending for `date_type: "new_awards_only"` instead of
// "action_date", which is USAspending's own documented way to exclude a
// modification to an older award (one whose action date moved but whose
// Base Obligation Date didn't); a description still has to contain a
// build word and none of an exclude list; at most one lead posts per
// recipient per run; and, when cfg.PreferSmallBusiness is on, a
// recipient's business categories (from the award's own detail page,
// since spending_by_award doesn't carry them) move a small-business
// recipient ahead of others rather than dropping anyone.
package usaspending

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

const Name = "usaspending-awards"

// vars, not consts, so tests can point them at a local fixture server.
var (
	apiURL             = "https://api.usaspending.gov/api/v2/search/spending_by_award/"
	awardDetailBaseURL = "https://api.usaspending.gov/api/v2/awards/"
)

// Config is the small set of filters this adapter applies; usaspending
// has no keyword list of its own, so config.go fills these in from
// bidwatch's config file rather than from [match] (which leads never run
// through at all; see runner.go).
type Config struct {
	WindowDays          int      // new awards with a Base Obligation Date in the trailing N days
	MinAmount           float64  // award amount floor, inclusive
	MaxAmount           float64  // award amount ceiling, inclusive
	NAICSCodes          []string // e.g. 541511, 541512, 541519, 518210
	IncludeWords        []string // the description needs at least one of these (whole-word match)
	ExcludeWords        []string // ...and none of these (whole-word match too; include a plural separately, e.g. "licenses", if it matters)
	PreferSmallBusiness bool     // sort a recipient flagged "small business" (or similar) ahead of others, rather than dropping the rest
}

type award struct {
	AwardID    string  `json:"Award ID"`
	Recipient  string  `json:"Recipient Name"`
	Amount     float64 `json:"Award Amount"`
	Agency     string  `json:"Awarding Agency"`
	Desc       string  `json:"Description"`
	InternalID string  `json:"generated_internal_id"`
}

type searchResponse struct {
	Results []award `json:"results"`
}

type awardDetail struct {
	Recipient struct {
		BusinessCategories []string `json:"business_categories"`
	} `json:"recipient"`
}

// candidate is an award past the description filter, carrying what the
// per-recipient cap and the small-business preference need.
type candidate struct {
	listing    model.Listing
	recipient  string
	internalID string
	small      bool
}

// New returns a Fetcher that posts one query to USAspending for new
// (not modified) awards in the trailing cfg.WindowDays, under
// cfg.NAICSCodes, with an amount between cfg.MinAmount and cfg.MaxAmount,
// whose description reads like build work.
func New(cfg Config) sources.Fetcher {
	return func(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error) {
		now := time.Now().UTC()
		start := now.AddDate(0, 0, -cfg.WindowDays)

		reqBody, err := json.Marshal(map[string]any{
			"filters": map[string]any{
				"award_type_codes": []string{"B", "C", "D"},
				"time_period": []map[string]string{{
					"start_date": start.Format("2006-01-02"),
					"end_date":   now.Format("2006-01-02"),
					// "new_awards_only" is USAspending's own documented
					// date_type for excluding a modification to an
					// existing award whose action date just moved; see
					// the 400 error naming the valid values
					// ('action_date', 'date_signed',
					// 'last_modified_date' or 'new_awards_only') for
					// where this came from.
					"date_type": "new_awards_only",
				}},
				"naics_codes": cfg.NAICSCodes,
				"award_amounts": []map[string]float64{{
					"lower_bound": cfg.MinAmount,
					"upper_bound": cfg.MaxAmount,
				}},
			},
			"fields": []string{
				"Award ID", "Recipient Name", "Award Amount",
				"Awarding Agency", "Description", "generated_internal_id",
				"Base Obligation Date",
			},
			"page":  1,
			"limit": 100,
			// Newest first, per the task: a stale award sitting at the
			// bottom of a 100-row page is as good as not fetched.
			"sort":  "Base Obligation Date",
			"order": "desc",
		})
		if err != nil {
			return nil, fmt.Errorf("%s: build request: %w", Name, err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("%s: build request: %w", Name, err)
		}
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", Name, err)
		}
		defer resp.Body.Close()

		raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err != nil {
			return nil, fmt.Errorf("%s: read response: %w", Name, err)
		}
		if resp.StatusCode != http.StatusOK {
			// A public API answering with an error is not the same as a
			// source refusing a request it recognized as a bot; this is
			// a plain error, not sources.BlockedError.
			return nil, fmt.Errorf("%s: HTTP %d from the award search API", Name, resp.StatusCode)
		}

		var parsed searchResponse
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return nil, fmt.Errorf("%s: parse response: %w", Name, err)
		}

		// Results arrive newest-first already; the order candidates are
		// built in is the order "first occurrence per recipient" below
		// keeps.
		var candidates []candidate
		seenRecipient := map[string]bool{}
		for _, a := range parsed.Results {
			if a.Recipient == "" || a.InternalID == "" {
				continue
			}
			if !passesDescriptionFilter(a.Desc, cfg.IncludeWords, cfg.ExcludeWords) {
				continue
			}
			recipientKey := strings.ToLower(strings.TrimSpace(a.Recipient))
			if seenRecipient[recipientKey] {
				// At most one lead per recipient per run; results are
				// newest-first, so the one already kept is the newest.
				continue
			}
			seenRecipient[recipientKey] = true

			link := "https://www.usaspending.gov/award/" + a.InternalID
			candidates = append(candidates, candidate{
				recipient:  a.Recipient,
				internalID: a.InternalID,
				listing: model.Listing{
					Source: Name,
					ID:     link,
					Kind:   model.KindLead,
					Title:  strings.TrimSpace(a.Recipient),
					Buyer:  strings.TrimSpace(a.Agency),
					Detail: fmt.Sprintf("%s award from %s: %s", formatAmount(a.Amount), strings.TrimSpace(a.Agency), shorten(a.Desc, 160)),
					URL:    link,
				},
			})
		}

		if cfg.PreferSmallBusiness {
			for i := range candidates {
				small, err := isSmallBusiness(ctx, client, ua, candidates[i].internalID)
				if err != nil {
					// Unknown, not false: a lookup that fails (a
					// timeout, a 404 on a since-corrected award) never
					// drops a candidate, it just doesn't get the
					// small-business boost this run.
					continue
				}
				candidates[i].small = small
			}
			sort.SliceStable(candidates, func(i, j int) bool {
				return candidates[i].small && !candidates[j].small
			})
		}

		out := make([]model.Listing, 0, len(candidates))
		for _, c := range candidates {
			out = append(out, c.listing)
		}
		return out, nil
	}
}

// passesDescriptionFilter reports whether desc reads like real build
// work: at least one whole-word hit from include, and no hit at all
// from exclude. Both sides match whole words only, the same \b-bounded
// check match.go's wordMatch uses for the RFP keyword lists, so "lab"
// never fires inside "labor" and "app" never fires inside "happen" or
// "appropriation"; it's why the default exclude list spells out
// "license" and "licenses" separately rather than relying on one to
// catch the other as a substring.
func passesDescriptionFilter(desc string, include, exclude []string) bool {
	for _, ex := range exclude {
		ex = strings.TrimSpace(ex)
		if ex == "" {
			continue
		}
		if wholeWordMatch(desc, ex) {
			return false
		}
	}
	for _, inc := range include {
		inc = strings.TrimSpace(inc)
		if inc == "" {
			continue
		}
		if wholeWordMatch(desc, inc) {
			return true
		}
	}
	return false
}

func wholeWordMatch(haystack, phrase string) bool {
	re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(phrase) + `\b`)
	if err != nil {
		return strings.Contains(strings.ToLower(haystack), strings.ToLower(phrase))
	}
	return re.MatchString(haystack)
}

// isSmallBusiness calls the award's own detail page (spending_by_award
// doesn't carry recipient business categories) and reports whether any
// category mentions "small", the broadest reasonable net over "Small
// Business", "Small Disadvantaged Business", "Economically
// Disadvantaged Women-Owned Small Business" and similar SBA categories.
func isSmallBusiness(ctx context.Context, client *http.Client, ua, generatedInternalID string) (bool, error) {
	u := awardDetailBaseURL + url.PathEscape(generatedInternalID) + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", ua)

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return false, err
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("HTTP %d from the award detail API", resp.StatusCode)
	}

	var d awardDetail
	if err := json.Unmarshal(raw, &d); err != nil {
		return false, err
	}
	for _, c := range d.Recipient.BusinessCategories {
		if strings.Contains(strings.ToLower(c), "small") {
			return true, nil
		}
	}
	return false, nil
}

func formatAmount(amount float64) string {
	whole := int64(amount + 0.5)
	s := fmt.Sprintf("%d", whole)
	// Comma-group the digits by hand: no fmt verb does this, and pulling
	// in a formatting dependency for one line isn't worth it.
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var grouped []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			grouped = append(grouped, ',')
		}
		grouped = append(grouped, c)
	}
	out := "$" + string(grouped)
	if neg {
		out = "-" + out
	}
	return out
}

func shorten(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "..."
}
