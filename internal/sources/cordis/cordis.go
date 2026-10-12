// Package cordis adapts CORDIS's free bulk open-data export (the HORIZON
// Projects CSV at cordis.europa.eu/data) to find recently started Horizon
// Europe research projects that look like they haven't set up a
// dissemination website yet. A Horizon Europe grant agreement requires a
// dissemination plan, and most multi-partner consortium projects
// (RIA/IA/CSA) set up a dedicated project website in their first few
// months; a project with nothing listed, or a listed site that's gone
// dead, is a direct, no-strings lead. Its coordinator (a university,
// research centre or company) is who to write to, the same
// "lead-to-approach, not a buyer to bid against" shape as the usaspending
// adapter: by the time a Horizon Europe grant is signed, there's nothing
// open to bid on.
//
// Data source, and why: researched in
// monetization/research/contracting/opposite.md (12 Oct 2026). CORDIS's
// own search UI (cordis.europa.eu/search) only renders its result list
// through a JavaScript call this adapter doesn't reproduce, so this reads
// CORDIS's documented bulk-data export instead, linked from the "CORDIS -
// EU research projects under HORIZON EUROPE" dataset on data.europa.eu
// (id cordis-eu-research-projects-under-horizon-europe-2021-2027). Of
// CORDIS's own export formats for the same data (CSV, JSON, XLSX, one zip
// each), CSV is the lightest: about 37 MB zipped against JSON's 141 MB or
// XLSX's own zip, while still carrying every field this adapter needs
// (project.csv, organization.csv, webLink.csv, inside one zip, one GET,
// no key, no auth). The zip's own Last-Modified header read 22 Sep 2026
// when this was written on 12 Oct 2026: CORDIS refreshes the export
// periodically, not live, so a run is always a few weeks behind CORDIS's
// own pages.
//
// IMPORTANT, read before turning sources.cordis_leads back on: this
// source shipped built and tested, but disabled by default
// (config.example.toml and Taktek's own config both set
// cordis_leads = false), because the data does not support the "empty
// website field means no website" premise it was built for.
//
// Checked 12 Oct 2026: webLink.csv's "relatedWebsite" rows (what this
// adapter reads as "a project website is listed") are an editorially
// curated field, not a systematic one. Counted by project start year,
// only 13% of 2021-start projects have one recorded, dropping to roughly
// 1-2% for 2022-2025 and under 0.2% for 2026-start projects, even though
// 2021-2023 projects are 3-5 years old now and overwhelmingly have real
// sites. A direct spot-check of 3 "no website listed" 2022 consortium
// projects (SolDAC, Photo2Fuel, SSH CENTRE) found all 3 already running
// real, live project websites (soldac-project.eu, photo2fuel.eu,
// sshcentre.eu) that CORDIS never recorded; each project's own CORDIS
// page confirms it too, showing a "Website" link for member
// organisations but never one for the project itself.
//
// That alone could be forgiven if the signal still held for the narrow
// slice this adapter actually targets: a SIGNED consortium project
// within WindowDays of its own startDate, where "genuinely too new to
// have built a site yet" is plausible on its own, independent of
// CORDIS's record-keeping. The one real run on 12 Oct 2026 (WindowDays
// 45, MaxPerRun 5) tested that directly, by hand, split by how old each
// eligible project actually was. The newest cohort (started 2026-10-01,
// 11 days old at run time: DigiWaves, FarmHeroes, PROCLAIM, EXCELY,
// RESSORT, the 5 the run actually posted) held up: a live web search
// found no dedicated project site for any of the 5. But the same run's
// much larger eligible pool before the per-run cap, the 2026-09-01
// cohort (41 days old, several hundred SIGNED consortium projects, every
// one of them also lacking a recorded website), told a different story:
// a 6-project spot-check (ARION, PostQat, OceanNexus, WUNDERWORK,
// EOSC4ALL, SHIFT 5.0) found 3 of 6 already running real, live
// dissemination websites (openaire.eu/arion-101287335, postqat.com,
// oceannexus.eu, live within 4 weeks of kickoff) that CORDIS had not
// recorded: a roughly 50% false-positive rate at the older end of the
// very window this source is configured to use by default. Narrowing
// WindowDays would only shrink the pool back to the handful-a-month
// "just signed" sliver (the scale opposite.md's own research already
// found: "5 genuine multi-partner ones pulled today"), not fix the
// underlying problem: CORDIS's field says almost nothing about whether a
// project has a site, only whether CORDIS got around to writing about
// it, so widening the window for useful volume just lets more
// already-built sites back in as false positives. Built and tested
// anyway, with a working one-GET/no-key data source, dedupe and cap, in
// case CORDIS ever starts recording this field systematically or a
// better per-project signal shows up elsewhere in its open data; only the
// premise behind "field empty" needs rechecking before flipping the
// config flag back on.
package cordis

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/taktekhq/bidwatch/internal/model"
	"github.com/taktekhq/bidwatch/internal/sources"
)

const Name = "cordis-leads"

// vars, not consts, so tests can point them at a local fixture server, or
// at a fixed clock.
var (
	dataURL = "https://cordis.europa.eu/data/cordis-HORIZONprojects-csv.zip"
	nowFunc = time.Now
)

// consortiumSchemes is the always-on funding-scheme filter: a solo ERC
// grant or an individual MSCA fellowship has no consortium to coordinate
// dissemination for, so "needs a project website" doesn't read the same
// way it does for a multi-partner RIA/IA/CSA project. Not a setting, the
// same way usaspending's one-lead-per-recipient cap isn't: a solo
// fellowship is never a good lead here regardless of any other filter.
var consortiumSchemes = map[string]bool{
	"HORIZON-RIA":    true,
	"HORIZON-IA":     true,
	"HORIZON-CSA":    true,
	"HORIZON-JU-RIA": true,
	"HORIZON-JU-IA":  true,
	"HORIZON-JU-CSA": true,
	"EURATOM-RIA":    true,
	"EURATOM-IA":     true,
	"EURATOM-CSA":    true,
}

// Config is the filter set applied to the bulk export; config.go fills
// this in from bidwatch's config file. Leads never run through [match]
// (see runner.go), so there's no keyword list here either, the same as
// usaspending.
type Config struct {
	WindowDays int // a SIGNED project with a startDate in the trailing N days
	// SkipCountries drops any project whose coordinator's country code
	// (ISO 3166-1 alpha-2, as CORDIS stores it) is in this list.
	// Default ["DE"]: Taktek's own cold-email rules skip Germany.
	SkipCountries []string
	// Programmes, if non-empty, keeps only a project whose legalBasis,
	// masterCall or subCall contains (case-insensitive substring) at
	// least one of these, e.g. "HORIZON-CL5" for the Climate, Energy and
	// Mobility cluster, or "HORIZON.1" for the Excellent Science pillar.
	// Empty means no filter: every Horizon Europe programme area.
	Programmes []string
	MaxPerRun  int // cap on how many leads this adapter returns, before the shared [leads].max_per_run cap
}

type projectRow struct {
	id, acronym, status, title, startDate string
	ecMaxContribution, fundingScheme      string
	legalBasis, masterCall, subCall       string
}

type coordinator struct {
	name, country string
}

// candidate is a project past every filter, carrying what the
// one-lead-per-coordinator-per-run cap needs.
type candidate struct {
	recipient string // lower-cased, trimmed coordinator name, for the cap
	startDate time.Time
	listing   model.Listing
}

// New returns a Fetcher that downloads CORDIS's bulk HORIZON-projects CSV
// export once, finds SIGNED consortium projects (RIA/IA/CSA and their JU
// and EURATOM equivalents) with a startDate in the trailing cfg.WindowDays
// days, drops any whose coordinator's country is in cfg.SkipCountries or
// that don't match cfg.Programmes, and keeps the rest only if CORDIS lists
// no project website for them or the one it lists doesn't resolve.
func New(cfg Config) sources.Fetcher {
	return func(ctx context.Context, client *http.Client, ua string) ([]model.Listing, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, dataURL, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: build request: %w", Name, err)
		}
		req.Header.Set("User-Agent", ua)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", Name, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: HTTP %d from CORDIS's bulk data export", Name, resp.StatusCode)
		}

		// The real export is about 37 MB zipped; 200 MB is a generous cap
		// against a truncated or wildly different response, not a tuned
		// estimate of the file's real size.
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 200<<20))
		if err != nil {
			return nil, fmt.Errorf("%s: read response: %w", Name, err)
		}

		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			return nil, fmt.Errorf("%s: open export zip: %w", Name, err)
		}

		projects, err := parseProjects(zr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", Name, err)
		}
		coordinators, err := parseCoordinators(zr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", Name, err)
		}
		websites, err := parseWebsites(zr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", Name, err)
		}

		now := nowFunc().UTC()
		windowStart := now.AddDate(0, 0, -cfg.WindowDays)

		var candidates []candidate
		for _, p := range projects {
			if p.status != "SIGNED" {
				continue
			}
			if !consortiumSchemes[strings.ToUpper(strings.TrimSpace(p.fundingScheme))] {
				continue
			}
			sd, err := time.Parse("2006-01-02", p.startDate)
			if err != nil {
				continue
			}
			if sd.Before(windowStart) || sd.After(now) {
				continue
			}
			if len(cfg.Programmes) > 0 && !matchesProgramme(p, cfg.Programmes) {
				continue
			}

			coord, ok := coordinators[p.id]
			if !ok || coord.name == "" {
				continue
			}
			if isSkippedCountry(coord.country, cfg.SkipCountries) {
				continue
			}

			reason, deadURL := "no website listed", ""
			if urls := websites[p.id]; len(urls) > 0 {
				if !headFails(ctx, client, ua, urls[0]) {
					continue // CORDIS lists a site and it resolves: not a lead
				}
				reason, deadURL = "site down", urls[0]
			}

			link := "https://cordis.europa.eu/project/id/" + p.id
			candidates = append(candidates, candidate{
				recipient: strings.ToLower(strings.TrimSpace(coord.name)),
				startDate: sd,
				listing: model.Listing{
					Source:  Name,
					ID:      link,
					Kind:    model.KindLead,
					Title:   strings.TrimSpace(coord.name),
					Country: coord.country,
					Detail:  buildDetail(p, coord, reason, deadURL),
					URL:     link,
				},
			})
		}

		// Newest start date first, so "at most one lead per coordinator
		// per run" below keeps that coordinator's newest qualifying
		// project, the same choice usaspending makes for a recipient with
		// more than one qualifying award.
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].startDate.After(candidates[j].startDate)
		})

		seen := map[string]bool{}
		var out []model.Listing
		for _, c := range candidates {
			if seen[c.recipient] {
				continue
			}
			seen[c.recipient] = true
			out = append(out, c.listing)
			if cfg.MaxPerRun > 0 && len(out) >= cfg.MaxPerRun {
				break
			}
		}
		return out, nil
	}
}

func buildDetail(p projectRow, coord coordinator, reason, deadURL string) string {
	status := reason
	if deadURL != "" {
		status = reason + ": " + deadURL
	}
	return fmt.Sprintf("coordinator of %s (%s), %s, %s Horizon Europe grant, started %s, %s",
		p.acronym, shorten(p.title, 80), coord.country, formatAmount(parseAmount(p.ecMaxContribution)), p.startDate, status)
}

func matchesProgramme(p projectRow, programmes []string) bool {
	hay := strings.ToLower(p.legalBasis + " " + p.masterCall + " " + p.subCall)
	for _, want := range programmes {
		want = strings.ToLower(strings.TrimSpace(want))
		if want != "" && strings.Contains(hay, want) {
			return true
		}
	}
	return false
}

func isSkippedCountry(country string, skip []string) bool {
	country = strings.ToUpper(strings.TrimSpace(country))
	for _, s := range skip {
		if strings.ToUpper(strings.TrimSpace(s)) == country {
			return true
		}
	}
	return false
}

// headFails sends one polite HEAD request with the honest, bot-naming
// user agent every other adapter sends, and reports whether the site
// failed to resolve: a network error, or any status 400 or above. It
// never retries and never falls back to a GET if the HEAD itself fails.
func headFails(ctx context.Context, client *http.Client, ua, rawURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return true
	}
	req.Header.Set("User-Agent", ua)

	resp, err := client.Do(req)
	if err != nil {
		return true
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 400
}

// parseProjects reads project.csv into a map keyed by CORDIS project ID.
func parseProjects(zr *zip.Reader) (map[string]projectRow, error) {
	header, rows, err := readCSV(zr, "project.csv")
	if err != nil {
		return nil, err
	}
	idx := indexer(header)
	out := make(map[string]projectRow, len(rows))
	for _, row := range rows {
		id := field(row, idx, "id")
		if id == "" {
			continue
		}
		out[id] = projectRow{
			id:                id,
			acronym:           field(row, idx, "acronym"),
			status:            field(row, idx, "status"),
			title:             field(row, idx, "title"),
			startDate:         field(row, idx, "startDate"),
			ecMaxContribution: field(row, idx, "ecMaxContribution"),
			fundingScheme:     field(row, idx, "fundingScheme"),
			legalBasis:        field(row, idx, "legalBasis"),
			masterCall:        field(row, idx, "masterCall"),
			subCall:           field(row, idx, "subCall"),
		}
	}
	return out, nil
}

// parseCoordinators reads organization.csv and keeps only each project's
// coordinator (role == "coordinator"): the lead to approach, never a
// participant or associated partner.
func parseCoordinators(zr *zip.Reader) (map[string]coordinator, error) {
	header, rows, err := readCSV(zr, "organization.csv")
	if err != nil {
		return nil, err
	}
	idx := indexer(header)
	out := make(map[string]coordinator, len(rows)/4)
	for _, row := range rows {
		if field(row, idx, "role") != "coordinator" {
			continue
		}
		pid := field(row, idx, "projectID")
		if pid == "" {
			continue
		}
		out[pid] = coordinator{
			name:    field(row, idx, "name"),
			country: strings.ToUpper(field(row, idx, "country")),
		}
	}
	return out, nil
}

// parseWebsites reads webLink.csv and keeps only the editorial
// "relatedWebsite" rows that represent the project itself (not a document,
// a social media link, or an organisation's own site), keyed by project
// ID; a project can have more than one, though in practice almost never
// does.
func parseWebsites(zr *zip.Reader) (map[string][]string, error) {
	header, rows, err := readCSV(zr, "webLink.csv")
	if err != nil {
		return nil, err
	}
	idx := indexer(header)
	out := map[string][]string{}
	for _, row := range rows {
		if field(row, idx, "type") != "relatedWebsite" || field(row, idx, "represents") != "project" {
			continue
		}
		pid := field(row, idx, "projectID")
		u := field(row, idx, "physUrl")
		if pid == "" || u == "" {
			continue
		}
		out[pid] = append(out[pid], u)
	}
	return out, nil
}

// readCSV finds name inside the export zip and parses it: CORDIS's CSV
// uses ";" as the field separator (commas appear inside decimal amounts
// and free-text fields), and LazyQuotes tolerates the occasional
// unescaped quote inside a long objective field rather than failing the
// whole file over one row.
func readCSV(zr *zip.Reader, name string) ([]string, [][]string, error) {
	var f *zip.File
	for _, zf := range zr.File {
		if zf.Name == name {
			f = zf
			break
		}
	}
	if f == nil {
		return nil, nil, fmt.Errorf("%s not found in the CORDIS export zip", name)
	}

	rc, err := f.Open()
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", name, err)
	}
	defer rc.Close()

	cr := csv.NewReader(rc)
	cr.Comma = ';'
	cr.LazyQuotes = true
	// CORDIS's own export has a handful of rows whose free-text fields
	// (seen in project.csv's "objective") carry a raw, unescaped
	// semicolon instead of a properly quoted one, shifting that one
	// row's later columns rather than breaking the file. FieldsPerRecord
	// -1 skips Go's strict same-width check so one bad row is read with
	// whatever columns it has (field() below already guards a short
	// row) instead of failing every row after it in the same file.
	cr.FieldsPerRecord = -1
	all, err := cr.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", name, err)
	}
	if len(all) == 0 {
		return nil, nil, fmt.Errorf("%s: empty", name)
	}
	return all[0], all[1:], nil
}

func indexer(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[h] = i
	}
	return m
}

func field(row []string, idx map[string]int, col string) string {
	i, ok := idx[col]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

// parseAmount reads CORDIS's own decimal style (a comma, e.g.
// "2073781,25") rather than a plain float string.
func parseAmount(s string) float64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func formatAmount(amount float64) string {
	whole := int64(amount + 0.5)
	s := fmt.Sprintf("%d", whole)
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
	out := "€" + string(grouped) // "€" is the euro sign
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
