package hostbridge

type ProjectWalkFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type ProjectWalkResult struct {
	Root      string            `json:"root"`
	Files     []ProjectWalkFile `json:"files"`
	Manifests []string          `json:"manifests,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
}
