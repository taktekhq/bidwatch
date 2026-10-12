// Package model holds the shared data shape every source adapter returns.
package model

import "time"

// KindLead marks a Listing as a lead rather than an open RFP: a company or
// site worth a direct, low-key note before any tender exists (an awarded
// contract to approach as a subcontractor, an end-of-life tech site). The
// zero value of Listing.Kind ("") means an ordinary RFP-board listing,
// which is every source bidwatch had before leads existed.
const KindLead = "lead"

// Listing is one open opportunity, or one lead, found on a source.
type Listing struct {
	Source  string // adapter name, e.g. "nten-jobs"
	ID      string // stable identifier for dedupe; the listing's own URL for an RFP, or the bare domain for a site lead
	Kind    string // "" for an RFP-board listing (the default), or KindLead
	Title   string // the RFP's title, or, for a lead, the thing to lead with: a recipient name or a domain
	Buyer   string // the agency or organisation behind an RFP; empty for most leads
	Country string // empty if the source doesn't say
	// Detail carries a lead's one-line context already formatted by its
	// adapter (award amount and agency, or the Drupal 7 evidence and the
	// date first seen). RFP listings leave it empty; the message's RFP
	// section never reads it.
	Detail   string
	Deadline *time.Time // nil when the source states no deadline or it could not be parsed; always nil for a lead
	URL      string
}
