package ir

// Plan represents a calculated execution plan.
type Plan struct {
	Metadata *PlanMetadata     `pkl:"metadata"`
	Changes  []*ResourceChange `pkl:"changes"`
	Summary  *PlanSummary      `pkl:"summary"`
	Outputs  map[string]any    `pkl:"outputs"`
}

type PlanMetadata struct {
	Timestamp      string  `pkl:"timestamp"`
	ConfigHash     string  `pkl:"configHash"`
	PriorStateHash *string `pkl:"priorStateHash"`
}

type ResourceChange struct {
	Address string                   `pkl:"address"`
	Action  string                   `pkl:"action"` // "create", "update", "delete", "replace", "noop"
	Desired *Resource                `pkl:"resource"`
	Prior   *Resource                `pkl:"prior"`
	Diff    map[string]*PropertyDiff `pkl:"diff"`
}

type PropertyDiff struct {
	Before            any    `pkl:"before"`
	After             any    `pkl:"after"`
	Sensitive         bool   `pkl:"sensitive"`
	ForcesReplacement bool   `pkl:"forcesReplacement"`
	Action            string `pkl:"action"` // "create", "update", "delete", "noop"
}

type PlanSummary struct {
	Create  int `pkl:"create"`
	Update  int `pkl:"update"`
	Delete  int `pkl:"delete"`
	Replace int `pkl:"replace"`
	NoOp    int `pkl:"noop"`
}
