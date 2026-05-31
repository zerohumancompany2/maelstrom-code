package assembly

type StaticSystemChunk struct {
	Prompt string
}

func (c StaticSystemChunk) Name() string { return "system-prompt" }

func (c StaticSystemChunk) Build(input Input) (ChunkResult, error) {
	step := ProvenanceStep{ChunkName: c.Name(), Operation: "project-system", OutputDescriptor: "system prompt"}
	return ChunkResult{Segments: []Segment{PromptSegment{Role: "system", Content: c.Prompt, Step: step}}, Steps: []ProvenanceStep{step}}, nil
}
