package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch7/catalog"
	"github.com/comalice/inference_sketch/sketch/sketch7/compile"
	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runner"
	"github.com/comalice/inference_sketch/sketch/sketch7/tools"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "sketch7: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: go run ./sketch/sketch7 [--model <model.yaml>] [--agent <agent.yaml>] [--workflow <workflow.yaml>] --prompt <text>")
	}
	args, err := parseArgs(os.Args[1:])
	if err != nil {
		return err
	}
	memory := catalog.NewMemory()
	for _, path := range []string{args.modelPath, args.agentPath, args.workflowPath} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := catalog.LoadIntoMemory(memory, raw); err != nil {
			return err
		}
	}
	modelDef, ok := firstModel(memory)
	if !ok {
		modelDef = defaultModelDefinition()
	}
	agentDef, ok := firstAgent(memory)
	if !ok {
		agentDef = defaultAgentDefinition(modelDef.Name)
	}
	var workflowDefPtr any
	_ = workflowDefPtr
	var workflowDef *struct{} // dummy to keep explicit nil path obvious
	_ = workflowDef
	toolRegistry := buildToolRegistry()
	hydratedAgent, err := compile.HydrateAgent(agentDef, modelDef, toolRegistry)
	if err != nil {
		return err
	}
	projections, maxHistory, err := prompt.BuildProjectionPlan(agentDef)
	if err != nil {
		return err
	}
	providerAdapter, err := providerFromEnv()
	if err != nil {
		return err
	}
	sessionHistory := logs.NewSessionHistory("session-001")
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: args.prompt})
	loop := runner.Loop{
		Provider:    providerAdapter,
		Tools:       toolRegistry,
		Projections: projections,
		MaxHistory:  maxHistory,
	}
	if err := loop.Run(hydratedAgent, agentDef, nil, sessionHistory, nil); err != nil {
		return err
	}
	for _, record := range sessionHistory.Records {
		fmt.Println(describeRecord(record))
	}
	return nil
}

type cliArgs struct {
	modelPath    string
	agentPath    string
	workflowPath string
	prompt       string
}

func parseArgs(args []string) (cliArgs, error) {
	parsed := cliArgs{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--model":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --model")
			}
			parsed.modelPath = args[i]
		case "--agent":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --agent")
			}
			parsed.agentPath = args[i]
		case "--workflow":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --workflow")
			}
			parsed.workflowPath = args[i]
		case "--prompt":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --prompt")
			}
			parsed.prompt = args[i]
		default:
			return cliArgs{}, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if parsed.prompt == "" {
		return cliArgs{}, fmt.Errorf("--prompt is required")
	}
	return parsed, nil
}

func defaultModelDefinition() defs.ModelDefinition {
	return defs.ModelDefinition{
		Name:      "zh-qwen36-27b-thinking",
		Providers: []defs.ProviderRef{{Name: "openai-compatible", ModelRef: "zh-qwen36-27b-thinking"}},
		Limits: defs.ModelLimits{
			ContextWindow:   32768,
			MaxOutputTokens: 8192,
		},
		Defaults: defs.ModelDefaults{
			Temperature: 0.7,
			TopP:        1.0,
		},
		Capabilities: defs.ModelCapabilities{
			Tools:      true,
			Reasoning:  true,
			Multimodal: false,
			Streaming:  false,
		},
	}
}

func defaultAgentDefinition(modelName string) defs.AgentDefinition {
	return defs.AgentDefinition{
		Name:        "maelstrom-code",
		Description: "Minimal sketch7 coding agent for local experiments",
		Model:       modelName,
		Tools: []string{
			"search_files",
			"get_file_skeleton",
			"read_symbol",
			"find_references",
			"read_file",
			"replace_text",
			"run_command",
		},
		Context: defs.ContextDefinition{
			InputBudget: 24000,
			Projections: []defs.ProjectionDefinition{
				{Type: "system", Name: "system", Prompt: "You are maelstrom-code, a precise coding agent. Prefer high-signal discovery before editing. Use the narrowest tool that can answer the question. Validate changes after editing."},
				{Type: "interaction"},
				{Type: "cognitive_state"},
				{Type: "messages"},
			},
		},
		Cognitive: defs.StatechartDefinition{
			InitialState: "observe",
			States: []defs.StateDefinition{
				{Name: "observe", Prompt: "Observe and gather evidence before acting.", VisibleTools: []string{"search_files", "get_file_skeleton", "read_symbol", "find_references", "read_file"}, EnabledTools: []string{"search_files", "get_file_skeleton", "read_symbol", "find_references", "read_file", "replace_text", "run_command"}},
			},
		},
	}
}

func providerFromEnv() (*provider.OpenAICompatibleProvider, error) {
	baseURL := strings.TrimSpace(os.Getenv("OPENAI_API_BASE"))
	if baseURL == "" {
		return nil, fmt.Errorf("OPENAI_API_BASE is required")
	}
	return &provider.OpenAICompatibleProvider{
		BaseURL: baseURL,
		APIKey:  strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
	}, nil
}

func buildToolRegistry() tools.Registry {
	root, _ := os.Getwd()
	return tools.NewRegistry(
		tools.BindingTool{},
		tools.UnbindTool{},
		tools.InterruptTool{},
		tools.ResumeTool{},
		tools.ReadFileTool{RootDir: root},
		tools.ReplaceTextTool{RootDir: root},
		tools.RunCommandTool{RootDir: root},
		tools.GetFileSkeletonTool{RootDir: root},
		tools.SearchFilesTool{RootDir: root},
		tools.ReadSymbolTool{RootDir: root},
		tools.FindReferencesTool{RootDir: root},
	)
}

func firstModel(memory *catalog.Memory) (defs.ModelDefinition, bool) {
	for _, def := range memory.Models {
		return def, true
	}
	return defs.ModelDefinition{}, false
}

func firstAgent(memory *catalog.Memory) (defs.AgentDefinition, bool) {
	for _, def := range memory.Agents {
		return def, true
	}
	return defs.AgentDefinition{}, false
}

func describeRecord(record logs.SessionRecord) string {
	switch v := record.(type) {
	case logs.UserMessageRecord:
		return fmt.Sprintf("user: %s", v.Content)
	case logs.AssistantMessageRecord:
		return fmt.Sprintf("assistant: %s", v.Content)
	case logs.ToolCallRequestRecord:
		return fmt.Sprintf("tool request: %s %s", v.ToolName, v.Arguments)
	case logs.ToolCallResultRecord:
		return fmt.Sprintf("tool result: %s\n%s", v.ToolName, v.Content)
	case logs.CognitiveTransitionRecord:
		return fmt.Sprintf("cognitive transition: %s -> %s via %s", v.FromState, v.ToState, v.Trigger)
	case logs.SessionWorkflowBindingRecord:
		return fmt.Sprintf("binding: %s %s", v.Action, v.WorkflowID)
	case logs.WorkflowTransitionRefRecord:
		return fmt.Sprintf("workflow transition: %s -> %s via %s", v.FromState, v.ToState, v.Trigger)
	case logs.InterruptRecord:
		return fmt.Sprintf("interrupt: %s", v.Reason)
	case logs.ResumeRecord:
		return fmt.Sprintf("resume: %s", v.Reason)
	default:
		return fmt.Sprintf("record: %T", record)
	}
}
