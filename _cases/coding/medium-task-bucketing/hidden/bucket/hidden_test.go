package bucket

import "testing"

// TestHiddenBucketing grades each artifact independently as a subtest so the
// harness can award per-artifact partial credit (see graderCommand/parseSubtests
// in internal/bench). Each subtest names its artifact so a partial score maps
// back to the specific artifact that failed.
func TestHiddenBucketing(t *testing.T) {
	projects := []Project{
		{ID: "atlas", Name: "Project Atlas", Code: "ATLAS", Aliases: []string{"Acme SSO", "Okta rollout"}},
		{ID: "nimbus", Name: "Project Nimbus", Code: "NIM", Aliases: []string{"usage billing", "metered invoices"}},
	}
	arts := []Artifact{
		{ID: "a", Text: "ACME SSO staging is ready for ATLAS-9"},
		{ID: "b", Text: "usage billing reconciliation for finance"},
		{ID: "c", Text: "The atlasian geography article is unrelated"},
		{ID: "d", Text: "ATLAS-2 and NIM-3 both mentioned"},
		{ID: "e", Text: "Okta rollout; Project Atlas launch checklist"},
	}
	want := []string{"atlas", "nimbus", "unrelated", "unrelated", "atlas"}
	got := BucketArtifacts(projects, arts)
	for i, w := range want {
		i, w := i, w
		t.Run(arts[i].ID, func(t *testing.T) {
			if i >= len(got) {
				t.Fatalf("missing assignment for artifact %q (got %d of %d)", arts[i].ID, len(got), len(want))
			}
			if got[i].ArtifactID != arts[i].ID || got[i].ProjectID != w {
				t.Errorf("artifact %q: got %#v, want project=%s", arts[i].ID, got[i], w)
			}
		})
	}
}
