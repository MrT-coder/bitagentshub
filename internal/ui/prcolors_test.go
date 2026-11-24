package ui

import (
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/office"
	"github.com/MrT-coder/bitagentshub/internal/prs"
)

func TestEachPullRequestStateHasItsColorAndShape(t *testing.T) {
	c := office.PR()
	for _, tc := range []struct {
		name  string
		pr    prs.PR
		glyph string
		want  func() bool
	}{
		{"open", prs.PR{State: "OPEN"}, "●", func() bool { return prStateStyle(prs.PR{State: "OPEN"}).Fg == c.Open }},
		{"draft", prs.PR{State: "OPEN", Draft: true}, "○", func() bool { return prStateStyle(prs.PR{Draft: true}).Fg == c.Draft }},
		{"merged", prs.PR{State: "MERGED"}, "◆", func() bool { return prStateStyle(prs.PR{State: "MERGED"}).Fg == c.Merged }},
		{"closed", prs.PR{State: "CLOSED"}, "✕", func() bool { return prStateStyle(prs.PR{State: "CLOSED"}).Fg == c.Closed }},
	} {
		if got := prStateGlyph(tc.pr); got != tc.glyph || !tc.want() {
			t.Errorf("%s: glyph %q (want %q) or color is not the theme's", tc.name, got, tc.glyph)
		}
	}
}

func TestTheTagsColorApprovalsChangesAndChecks(t *testing.T) {
	c := office.PR()
	tags := prTagSpans(prs.PR{State: "OPEN", Approvers: []string{"bob"}, ChangesBy: []string{"cy"}, Checks: "FAILURE"})
	want := []struct {
		text string
		fg   func() bool
	}{
		{"  ✓ bob", func() bool { return tags[0].style.Fg == c.Open }},
		{"  ✗ cy", func() bool { return tags[1].style.Fg == c.Closed }},
		{"  CI ✗", func() bool { return tags[2].style.Fg == c.Closed }},
	}
	if len(tags) != len(want) {
		t.Fatalf("tags %+v, want %d", tags, len(want))
	}
	for i, w := range want {
		if tags[i].text != w.text || !w.fg() {
			t.Errorf("tag %d = %q, want %q in its theme color", i, tags[i].text, w.text)
		}
	}
}

func TestAMergedPullRequestShowsNoChecksAndSaysMerged(t *testing.T) {
	tags := prTagSpans(prs.PR{State: "MERGED", Checks: "PENDING"})
	if len(tags) != 1 || tags[0].text != "  merged" {
		t.Errorf("tags %+v, want only \"merged\"", tags)
	}
}
