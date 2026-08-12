package association

type Project struct {
	ID      string
	Name    string
	Code    string
	Aliases []string
	People  []string
}

type Artifact struct {
	ID       string
	ParentID string
	Kind     string
	Text     string
}

type Result struct {
	ArtifactID string
	ProjectID  string
	Score      int
	Reason     string
}
