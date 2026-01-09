package ir

// Config represents the top-level configuration.
type Config struct {
	Providers map[string]any `pkl:"providers"`
	Resources []*Resource    `pkl:"resources"`
	Outputs   map[string]any `pkl:"outputs"`
}
