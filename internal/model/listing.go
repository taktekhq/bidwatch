// Package model holds the shared data shape every source adapter returns.
package model

import "time"

// Listing is one open opportunity found on a source.
type Listing struct {
	Source   string // adapter name, e.g. "nten-jobs"
	ID       string // stable identifier for dedupe; the listing's own URL
	Title    string
	Buyer    string
	Country  string     // empty if the source doesn't say
	Deadline *time.Time // nil when the source states no deadline or it could not be parsed
	URL      string
}
