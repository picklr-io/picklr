package ir

// Config represents the top-level configuration.
type Config struct {
	Resources []*Resource    `pkl:"resources"`
	Outputs   map[string]any `pkl:"outputs"`
}
