package bind

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch6/agent"
	"github.com/comalice/inference_sketch/sketch/sketch6/assembly"
	"github.com/comalice/inference_sketch/sketch/sketch6/inference"
	"github.com/comalice/inference_sketch/sketch/sketch6/provider"
	"github.com/comalice/inference_sketch/sketch/sketch6/runner"
	"github.com/comalice/inference_sketch/sketch/sketch6/tools"
)

type EnvResolver interface {
	Resolve(ctx context.Context, value string) (string, error)
}

type OSEnvResolver struct{}

func (OSEnvResolver) Resolve(_ context.Context, value string) (string, error) {
	return os.Expand(value, func(key string) string { return os.Getenv(key) }), nil
}

type BoundAgent struct {
	Spec agent.Spec
	Loop runner.Loop
}

type AgentBinder struct {
	Resolver EnvResolver
	Provider provider.Provider
	Tools    tools.Executor
	Recorder inference.Recorder
}

func (b AgentBinder) Bind(ctx context.Context, spec agent.Spec) (BoundAgent, error) {
	resolvedSpec, err := b.resolveSpec(ctx, spec)
	if err != nil {
		return BoundAgent{}, err
	}
	loop := runner.Loop{
		Assembler: assembly.Assembler{Chunks: []assembly.Chunk{
			assembly.SystemPromptChunk{},
			assembly.RecentHistoryChunk{},
			assembly.StateProjectionChunk{ChartName: "agent"},
			assembly.StateProjectionChunk{ChartName: "workflow"},
		}},
		Provider: b.Provider,
		Tools:    b.Tools,
		Recorder: b.Recorder,
	}
	return BoundAgent{Spec: resolvedSpec, Loop: loop}, nil
}

func (b AgentBinder) resolveSpec(ctx context.Context, spec agent.Spec) (agent.Spec, error) {
	resolver := b.Resolver
	if resolver == nil {
		resolver = OSEnvResolver{}
	}
	providerName, err := resolver.Resolve(ctx, spec.Model.Provider)
	if err != nil {
		return agent.Spec{}, fmt.Errorf("resolve provider: %w", err)
	}
	modelName, err := resolver.Resolve(ctx, spec.Model.Name)
	if err != nil {
		return agent.Spec{}, fmt.Errorf("resolve model: %w", err)
	}
	toolNames := make([]string, 0, len(spec.ToolNames))
	for _, toolName := range spec.ToolNames {
		resolved, err := resolver.Resolve(ctx, toolName)
		if err != nil {
			return agent.Spec{}, fmt.Errorf("resolve tool name %q: %w", toolName, err)
		}
		toolNames = append(toolNames, strings.TrimSpace(resolved))
	}
	spec.Model.Provider = strings.TrimSpace(providerName)
	spec.Model.Name = strings.TrimSpace(modelName)
	spec.ToolNames = toolNames
	return spec, nil
}
