package pet

import (
	"slices"
	"testing"
)

func TestReviewsWaitingSayWhyButDoNotChangeTheMood(t *testing.T) {
	for _, tc := range []struct {
		prs  int
		want string
	}{
		{1, "1 pull request waits for your review"},
		{3, "3 pull requests wait for your review"},
	} {
		st := Read(Telemetry{Working: 1, PRs: tc.prs})
		if st.Mood != Busy {
			t.Errorf("%d reviews waiting: mood %v, want Busy as without them", tc.prs, st.Mood)
		}
		if !slices.Contains(st.Why, tc.want) {
			t.Errorf("%d reviews waiting: why = %q, want it to say %q", tc.prs, st.Why, tc.want)
		}
	}
	for _, line := range Read(Telemetry{Working: 1}).Why {
		if line == "0 pull requests wait for your review" {
			t.Error("the state speaks of reviews although none waits")
		}
	}
}
