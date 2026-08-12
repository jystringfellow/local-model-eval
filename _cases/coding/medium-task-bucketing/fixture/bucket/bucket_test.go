package bucket

import "testing"

func TestExplicitCode(t *testing.T) {
	projects := []Project{{ID: "atlas", Name: "Project Atlas", Code: "ATLAS", Aliases: []string{"Acme SSO"}}}
	got := BucketArtifacts(projects, []Artifact{{ID: "1", Text: "fix ATLAS-42 audience validation"}})
	if len(got) != 1 || got[0].ProjectID != "atlas" {
		t.Fatalf("got %#v", got)
	}
}
