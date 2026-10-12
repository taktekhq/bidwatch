// Package runner ties the pieces together: fetch every configured source,
// apply the match rules, work out what's new against state, and build the
// one Slack message a run posts. It takes the source list as a parameter
// so it never imports a specific adapter itself, which keeps it testable
// with fakes.
package runner

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/taktekhq/bidwatch/internal/config"
	"github.com/taktekhq/bidwatch/internal/match"
	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

// SourceCount is what the run found at one source, for the per-source
// visibility line every message carries.
type SourceCount struct {
	Name    string
	Fetched int
	Ignored int // dropped by [ignore].substrings before matching; not counted in Matched/New
	Matched int
	New     int
	Credits int    // metered API credits this source spent (e.g. Firecrawl); 0 for every plain-HTTP source
	Blocked string // non-empty if the source was blocked; Fetched/Matched/New are all 0 then
	Err     string // non-empty on an unexpected error other than a block
}

// Summary is what one run produced.
type Summary struct {
	Counts      []SourceCount
	New         []model.Listing // matched RFP listings not already in state, across all sources
	NewLeads    []model.Listing // matched lead listings not already in state, before the per-run cap
	PostedLeads []model.Listing // NewLeads after the cap; what the message actually shows and what gets marked seen
	Message     string
}

// Seen is the subset of state.State a run needs; defined here so runner
// doesn't need to know about the on-disk format.
type Seen interface {
	IsNew(id string) bool
}

// Run fetches every source in srcs, in order, with cfg.DelayBetweenSources
// seconds of polite pause between each one. now is passed in rather than
// read from time.Now so tests are deterministic.
func Run(ctx context.Context, cfg *config.Config, seen Seen, srcs []sources.Source, client *http.Client, now time.Time) Summary {
	rules := match.Rules{Keywords: cfg.Match.Keywords, Excludes: cfg.Match.Excludes}

	var counts []SourceCount
	var newRFPs []model.Listing
	var newLeads []model.Listing

	for i, src := range srcs {
		if i > 0 {
			time.Sleep(time.Duration(cfg.DelayBetweenSources) * time.Second)
		}

		sc := SourceCount{Name: src.Name}
		var credits int
		fetchCtx := sources.WithCredits(ctx, &credits)
		listings, err := src.Fetch(fetchCtx, client, cfg.UserAgent)
		sc.Credits = credits
		if err != nil {
			var be *sources.BlockedError
			if errors.As(err, &be) {
				sc.Blocked = be.Reason
			} else {
				sc.Err = err.Error()
			}
			counts = append(counts, sc)
			continue
		}

		sc.Fetched = len(listings)
		for _, l := range listings {
			if isIgnored(l, cfg.Ignore.Substrings) {
				sc.Ignored++
				continue
			}
			if l.Kind == model.KindLead {
				// A lead's own adapter already applied its source-specific
				// filter (an amount band and NAICS codes, or a Drupal 7
				// site-code match): bidwatch's keyword/exclude rules are
				// for RFP boards and don't apply here.
				sc.Matched++
				if seen.IsNew(l.ID) {
					sc.New++
					newLeads = append(newLeads, l)
				}
				continue
			}
			res := match.Check(l, "", rules, now)
			if !res.Matched {
				continue
			}
			sc.Matched++
			if seen.IsNew(l.ID) {
				sc.New++
				newRFPs = append(newRFPs, l)
			}
		}
		counts = append(counts, sc)
	}

	sort.Slice(newRFPs, func(i, j int) bool {
		di, dj := newRFPs[i].Deadline, newRFPs[j].Deadline
		if di == nil && dj == nil {
			return newRFPs[i].Title < newRFPs[j].Title
		}
		if di == nil {
			return false
		}
		if dj == nil {
			return true
		}
		return di.Before(*dj)
	})
	sort.Slice(newLeads, func(i, j int) bool {
		if newLeads[i].Source != newLeads[j].Source {
			return newLeads[i].Source < newLeads[j].Source
		}
		return newLeads[i].Title < newLeads[j].Title
	})

	postedLeads := newLeads
	if cfg.Leads.MaxPerRun > 0 && len(postedLeads) > cfg.Leads.MaxPerRun {
		postedLeads = postedLeads[:cfg.Leads.MaxPerRun]
	}

	return Summary{
		Counts:      counts,
		New:         newRFPs,
		NewLeads:    newLeads,
		PostedLeads: postedLeads,
		Message:     formatMessage(newRFPs, postedLeads, len(newLeads), counts),
	}
}

// isIgnored reports whether l's title or URL contains any of substrings,
// case-insensitively: a standing "no" from Nizar, regardless of which
// source lists it or under what URL.
func isIgnored(l model.Listing, substrings []string) bool {
	if len(substrings) == 0 {
		return false
	}
	title := strings.ToLower(l.Title)
	url := strings.ToLower(l.URL)
	for _, s := range substrings {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if strings.Contains(title, s) || strings.Contains(url, s) {
			return true
		}
	}
	return false
}

func formatMessage(newRFPs []model.Listing, postedLeads []model.Listing, totalNewLeads int, counts []SourceCount) string {
	var b strings.Builder
	if len(newRFPs) == 0 {
		b.WriteString("bidwatch: no new matches today\n")
	} else {
		fmt.Fprintf(&b, "bidwatch: %d new match(es) today\n", len(newRFPs))
		for _, l := range newRFPs {
			deadline := "no stated deadline"
			if l.Deadline != nil {
				deadline = l.Deadline.Format("2 Jan 2006")
			}
			buyer := l.Buyer
			if buyer == "" {
				buyer = "buyer not stated"
			}
			fmt.Fprintf(&b, "- %s, %s, deadline %s, %s\n", l.Title, buyer, deadline, l.URL)
		}
	}

	b.WriteString("\nLeads (no RFP): ")
	if totalNewLeads == 0 {
		b.WriteString("no new leads\n")
	} else {
		fmt.Fprintf(&b, "%d new\n", len(postedLeads))
		for _, l := range postedLeads {
			fmt.Fprintf(&b, "- %s, %s, %s\n", l.Title, l.Detail, l.URL)
		}
		if held := totalNewLeads - len(postedLeads); held > 0 {
			fmt.Fprintf(&b, "(%d more new lead(s) held back this run, capped at %d; they carry over to a future run)\n", held, len(postedLeads))
		}
	}

	b.WriteString("\nSources: ")
	parts := make([]string, 0, len(counts))
	for _, c := range counts {
		switch {
		case c.Blocked != "":
			parts = append(parts, fmt.Sprintf("%s blocked (%s)", c.Name, c.Blocked))
		case c.Err != "":
			parts = append(parts, fmt.Sprintf("%s error (%s)", c.Name, c.Err))
		default:
			part := fmt.Sprintf("%s %d fetched, %d matched, %d new", c.Name, c.Fetched, c.Matched, c.New)
			if c.Ignored > 0 {
				part += fmt.Sprintf(", %d ignored", c.Ignored)
			}
			if c.Credits > 0 {
				part += fmt.Sprintf(", %d credit(s)", c.Credits)
			}
			parts = append(parts, part)
		}
	}
	b.WriteString(strings.Join(parts, "; "))

	// Belt and suspenders, on top of every adapter's own plain-comma
	// formatting: nothing posted to Slack is ever allowed to carry an em
	// dash, even if a source's own copied text (a title, a description)
	// happened to contain one.
	return strings.ReplaceAll(b.String(), "—", ", ")
}
