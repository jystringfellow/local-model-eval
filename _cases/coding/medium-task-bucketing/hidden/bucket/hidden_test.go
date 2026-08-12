package bucket

import "testing"

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
	got := BucketArtifacts(projects, arts)
	want := []string{"atlas", "nimbus", "unrelated", "unrelated", "atlas"}
	if len(got) != len(want) {
		t.Fatalf("len=%d", len(got))
	}
	for i := range want {
		if got[i].ArtifactID != arts[i].ID || got[i].ProjectID != want[i] {
			t.Errorf("%d got %#v want project=%s", i, got[i], want[i])
		}
	}
}
