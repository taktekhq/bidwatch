package match

import (
	"testing"
	"time"

	"github.com/taktekhq/bidwatch/internal/model"
)

var rules = Rules{
	Keywords: []string{"website", "software", "crm"},
	Excludes: []string{"construction", "catering"},
}

func at(t *testing.T, s string) time.Time {
	tm, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatal(err)
	}
	return tm
}

func deadline(t *testing.T, s string) *time.Time {
	tm := at(t, s)
	return &tm
}

func TestCheck_KeywordMatches(t *testing.T) {
	l := model.Listing{Title: "Website Redesign and Development (RFP)"}
	res := Check(l, "", rules, at(t, "2026-10-12"))
	if !res.Matched {
		t.Fatalf("expected a match, got %+v", res)
	}
	if res.Keyword != "website" {
		t.Errorf("Keyword = %q, want %q", res.Keyword, "website")
	}
}

func TestCheck_ExcludeWordWins(t *testing.T) {
	l := model.Listing{Title: "Website and construction of a new office"}
	res := Check(l, "", rules, at(t, "2026-10-12"))
	if res.Matched {
		t.Fatalf("expected no match (excluded by 'construction'), got %+v", res)
	}
	if res.Excluded != "construction" {
		t.Errorf("Excluded = %q", res.Excluded)
	}
}

func TestCheck_NoKeywordHit(t *testing.T) {
	l := model.Listing{Title: "Catering services for a conference"}
	res := Check(l, "", rules, at(t, "2026-10-12"))
	if res.Matched {
		t.Fatalf("expected no match, got %+v", res)
	}
}

func TestCheck_PastDeadlineFailsEvenWithKeyword(t *testing.T) {
	l := model.Listing{Title: "CRM Implementation", Deadline: deadline(t, "2026-01-01")}
	res := Check(l, "", rules, at(t, "2026-10-12"))
	if res.Matched {
		t.Fatalf("expected no match for a past deadline, got %+v", res)
	}
	if !res.PastDue {
		t.Errorf("expected PastDue true, got %+v", res)
	}
}

func TestCheck_FutureDeadlinePasses(t *testing.T) {
	l := model.Listing{Title: "CRM Implementation", Deadline: deadline(t, "2026-11-01")}
	res := Check(l, "", rules, at(t, "2026-10-12"))
	if !res.Matched {
		t.Fatalf("expected a match for a future deadline, got %+v", res)
	}
}

func TestCheck_UnknownDeadlinePasses(t *testing.T) {
	l := model.Listing{Title: "Software platform build", Deadline: nil}
	res := Check(l, "", rules, at(t, "2026-10-12"))
	if !res.Matched {
		t.Fatalf("expected a match when the deadline is unknown, got %+v", res)
	}
}

func TestCheck_MatchesAgainstDescriptionToo(t *testing.T) {
	l := model.Listing{Title: "Request for Proposal"}
	res := Check(l, "We need a new CRM for our sales team", rules, at(t, "2026-10-12"))
	if !res.Matched {
		t.Fatalf("expected a match from the description, got %+v", res)
	}
}

func TestCheck_ShortKeywordDoesNotMatchInsideALongerWord(t *testing.T) {
	// "ai" as a configured keyword; "Main" contains "ai" as a substring
	// but is not the word "AI". A real SBA SubNet listing ("Alvarado
	// Trunk Sewer & Water Main Relocations") matched on this before the
	// word-boundary fix.
	aiRules := Rules{Keywords: []string{"ai"}}
	l := model.Listing{Title: "Alvarado Trunk Sewer & Water Main Relocations"}
	res := Check(l, "", aiRules, at(t, "2026-10-12"))
	if res.Matched {
		t.Fatalf("expected no match: 'ai' inside 'Main' is not the word AI, got %+v", res)
	}
}

func TestCheck_ShortKeywordDoesMatchAsItsOwnWord(t *testing.T) {
	aiRules := Rules{Keywords: []string{"ai"}}
	l := model.Listing{Title: "AI Engineer for a chatbot"}
	res := Check(l, "", aiRules, at(t, "2026-10-12"))
	if !res.Matched {
		t.Fatalf("expected a match: AI appears as its own word, got %+v", res)
	}
}

func TestCheck_CaseInsensitive(t *testing.T) {
	l := model.Listing{Title: "WEBSITE REDESIGN"}
	res := Check(l, "", rules, at(t, "2026-10-12"))
	if !res.Matched {
		t.Fatalf("expected a case-insensitive match, got %+v", res)
	}
}
