package ir

// Resource represents a single managed resource.
type Resource struct {
	Type       string         `pkl:"type"` // e.g., "aws.s3.Bucket"
	Name       string         `pkl:"name"`
	Provider   string         `pkl:"provider"`
	Lifecycle  *Lifecycle     `pkl:"lifecycle"`
	DependsOn  []string       `pkl:"dependsOn"`
	Properties map[string]any `pkl:"properties"` // Dynamic properties
}

type Lifecycle struct {
	CreateBeforeDestroy bool     `pkl:"createBeforeDestroy"`
	PreventDestroy      bool     `pkl:"preventDestroy"`
	IgnoreChanges       []string `pkl:"ignoreChanges"`
}
