package bucket

type Project struct {
	ID      string
	Name    string
	Code    string
	Aliases []string
}

type Artifact struct {
	ID   string
	Text string
}

type Assignment struct {
	ArtifactID string
	ProjectID  string
}

func BucketArtifacts(projects []Project, artifacts []Artifact) []Assignment {
	panic("TODO")
}
