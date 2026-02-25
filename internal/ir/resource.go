package ir

// Resource represents a single managed resource.
type Resource struct {
	Type       string            `pkl:"type" json:"type"` // e.g., "aws:S3.Bucket"
	Name       string            `pkl:"name" json:"name"`
	Provider   string            `pkl:"provider" json:"provider"`
	Lifecycle  *Lifecycle        `pkl:"lifecycle" json:"lifecycle,omitempty"`
	DependsOn  []string          `pkl:"dependsOn" json:"depends_on,omitempty"`
	Properties map[string]any    `pkl:"properties" json:"properties"` // Dynamic properties
	Count      int               `pkl:"count" json:"count,omitempty"`        // Create N instances
	ForEach    map[string]any    `pkl:"forEach" json:"for_each,omitempty"`   // Create instance per key
	Timeout    string            `pkl:"timeout" json:"timeout,omitempty"`    // Per-resource timeout (e.g. "30m")
}

type Lifecycle struct {
	CreateBeforeDestroy bool     `pkl:"createBeforeDestroy"`
	PreventDestroy      bool     `pkl:"preventDestroy"`
	IgnoreChanges       []string `pkl:"ignoreChanges"`
}
