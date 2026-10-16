package association

import (
	"strings"
	"testing"
)

// TestHiddenResolution grades each artifact independently as a subtest so the
// harness can award per-artifact partial credit. The thread-parent inheritance
// reason is asserted on the "e" subtest.
func TestHiddenResolution(t *testing.T) {
	projects := []Project{
		{ID: "atlas", Name: "Project Atlas", Code: "ATLAS", Aliases: []string{"Acme SSO", "Okta rollout"}, People: []string{"Priya", "Marco"}},
		{ID: "nimbus", Name: "Project Nimbus", Code: "NIM", Aliases: []string{"usage billing", "metered invoices"}, People: []string{"Elena", "Sam"}},
	}
	arts := []Artifact{
		{ID: "a", Kind: "slack", Text: "Priya says Acme SSO staging is ready"},
		{ID: "b", Kind: "commit", Text: "fix ATLAS-42 audience validation"},
		{ID: "c", Kind: "email", Text: "Sam asks Elena about usage billing reconciliation"},
		{ID: "d", Kind: "meeting", Text: "Priya and Sam reviewed launch timing"},
		{ID: "e", ParentID: "a", Kind: "slack", Text: "I can handle the staging change"},
		{ID: "f", ParentID: "a", Kind: "slack", Text: "NIM-7 is also blocked"},
		{ID: "g", Kind: "email", Text: "The atlasian map is attached"},
	}
	want := []string{"atlas", "atlas", "nimbus", "unrelated", "atlas", "nimbus", "unrelated"}
	got := Resolve(projects, arts)
	for i, w := range want {
		i, w := i, w
		t.Run(arts[i].ID, func(t *testing.T) {
			if i >= len(got) {
				t.Fatalf("missing result for artifact %q (got %d of %d)", arts[i].ID, len(got), len(want))
			}
			if got[i].ArtifactID != arts[i].ID || got[i].ProjectID != w {
				t.Errorf("artifact %q: got %#v, want project=%s", arts[i].ID, got[i], w)
			}
			if arts[i].ID == "e" && !strings.Contains(got[i].Reason, "thread-parent") {
				t.Errorf("inherit reason=%q, want to contain 'thread-parent'", got[i].Reason)
			}
		})
	}
}

// TestHiddenAmbiguityMargin is a subtest too so the tie-break rule is part of
// the graded count rather than a check that escapes partial-credit scoring.
func TestHiddenAmbiguityMargin(t *testing.T) {
	projects := []Project{
		{ID: "a", Name: "Project A", Aliases: []string{"shared"}},
		{ID: "b", Name: "Project B", Aliases: []string{"shared"}},
	}
	got := Resolve(projects, []Artifact{{ID: "x", Text: "shared"}})
	t.Run("shared-alias-tie", func(t *testing.T) {
		if len(got) == 0 || got[0].ProjectID != "unrelated" {
			t.Fatalf("got %#v, want project=unrelated", got)
		}
	})
}
