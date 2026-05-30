package assembly

import (
	"fmt"
	"strings"
)

type SystemPromptChunk struct{}

func (SystemPromptChunk) Name() string { return "system-prompt" }

func (SystemPromptChunk) Build(input Input) (ChunkResult, error) {
	content := fmt.Sprintf("You are %s version %s on model %s. Tools: %s.", input.Agent.ID, input.Agent.Version, input.Agent.Model.Name, joinNames(input.Agent.ToolNames))
	step := ProvenanceStep{ChunkName: "system-prompt", Operation: "synthesize", OutputDescriptor: "system prompt"}
	return ChunkResult{Segments: []Segment{PromptSegment{Role: "system", Content: content, Step: step}}, Steps: []ProvenanceStep{step}}, nil
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}
