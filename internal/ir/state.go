package ir

// State represents the persistent state.
type State struct {
	Version   int              `pkl:"version"`
	Serial    int              `pkl:"serial"`
	Lineage   string           `pkl:"lineage"`
	Resources []*ResourceState `pkl:"resources"`
	Outputs   map[string]any   `pkl:"outputs"`
}

type ResourceState struct {
	Type         string         `pkl:"type"`
	Name         string         `pkl:"name"`
	Provider     string         `pkl:"provider"`
	Inputs       map[string]any `pkl:"inputs"` // User provided
	InputsHash   string         `pkl:"inputsHash"`
	Outputs      map[string]any `pkl:"outputs"` // Provider returned
	Dependencies []string       `pkl:"dependencies"`
}
