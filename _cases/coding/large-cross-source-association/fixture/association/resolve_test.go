package association

import "testing"

func TestDirectNameMatch(t *testing.T) {
	p := []Project{{ID: "atlas", Name: "Project Atlas", Code: "ATLAS"}}
	got := Resolve(p, []Artifact{{ID: "a", Kind: "slack", Text: "Project Atlas launch is Friday"}})
	if len(got) != 1 || got[0].ProjectID != "atlas" || got[0].Score < 5 {
		t.Fatalf("got %#v", got)
	}
}
