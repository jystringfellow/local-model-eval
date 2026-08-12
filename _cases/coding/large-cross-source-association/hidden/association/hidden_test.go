package association

import (
	"strings"
	"testing"
)

func TestHiddenResolution(t *testing.T) {
	projects := []Project{
		{ID: "atlas", Name: "Project Atlas", Code: "ATLAS", Aliases: []string{"Acme SSO", "Okta rollout"}, People: []string{"Priya", "Marco"}},
		{ID: "nimbus", Name: "Project Nimbus", Code: "NIM", Aliases: []string{"usage billing", "metered invoices"}, People: []string{"Elena", "Sam"}},
	}
	arts := []Artifact{
		{ID: "a", Kind: "slack", Text: "Priya says Acme SSO staging is ready"},              // 2+3 = 5 atlas
		{ID: "b", Kind: "commit", Text: "fix ATLAS-42 audience validation"},                 // 5 atlas
		{ID: "c", Kind: "email", Text: "Sam asks Elena about usage billing reconciliation"}, // 2+2+3 = 7 nimbus
		{ID: "d", Kind: "meeting", Text: "Priya and Sam reviewed launch timing"},            // tie/low => unrelated
		{ID: "e", ParentID: "a", Kind: "slack", Text: "I can handle the staging change"},    // inherit atlas
		{ID: "f", ParentID: "a", Kind: "slack", Text: "NIM-7 is also blocked"},              // explicit other => no inherit, nimbus direct
		{ID: "g", Kind: "email", Text: "The atlasian map is attached"},                      // boundary protection
	}
	got := Resolve(projects, arts)
	want := []string{"atlas", "atlas", "nimbus", "unrelated", "atlas", "nimbus", "unrelated"}
	if len(got) != len(want) {
		t.Fatalf("len %d", len(got))
	}
	for i, w := range want {
		if got[i].ArtifactID != arts[i].ID || got[i].ProjectID != w {
			t.Errorf("%s got %#v want %s", arts[i].ID, got[i], w)
		}
	}
	if !strings.Contains(got[4].Reason, "thread-parent") {
		t.Errorf("inherit reason=%q", got[4].Reason)
	}
}

func TestAmbiguityMargin(t *testing.T) {
	projects := []Project{
		{ID: "a", Name: "Project A", Aliases: []string{"shared"}},
		{ID: "b", Name: "Project B", Aliases: []string{"shared"}},
	}
	got := Resolve(projects, []Artifact{{ID: "x", Text: "shared"}})
	if got[0].ProjectID != "unrelated" {
		t.Fatalf("got %#v", got[0])
	}
}
