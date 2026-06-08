package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
		return fmt.Errorf("usage: go run ./sketch/sketch7 [--model <model.yaml>] [--agent <agent.yaml>] [--workflow <workflow.yaml>] [--session-id <id> | --state <path>] [--stop-token <token>] --prompt <text>")
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
	workflowDef, hasWorkflow := firstWorkflow(memory)
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
	statePath := strings.TrimSpace(args.statePath)
	if statePath == "" && strings.TrimSpace(args.sessionID) != "" {
		statePath = filepath.Join(".maelstrom", "sessions", args.sessionID+".json")
	}
	var sessionHistory *logs.SessionHistory
	var workflowHistory *logs.WorkflowHistory
	if statePath != "" {
		if _, err := os.Stat(statePath); err == nil {
			loadedSession, loadedWorkflow, err := logs.LoadState(statePath)
			if err != nil {
				return err
			}
			sessionHistory = loadedSession
			workflowHistory = loadedWorkflow
		}
	}
	if sessionHistory == nil {
		sessionID := strings.TrimSpace(args.sessionID)
		if sessionID == "" {
			sessionID = "session-001"
		}
		sessionHistory = logs.NewSessionHistory(sessionID)
	}
	if hasWorkflow && workflowHistory == nil {
		workflowID := sessionHistory.SessionID + ":" + workflowDef.Name
		workflowHistory = logs.NewWorkflowHistory(workflowID)
	}
	if hasWorkflow && !hasBindingRecord(sessionHistory) {
		bindingID := "bind-" + sessionHistory.SessionID
		sessionHistory.Append(logs.SessionWorkflowBindingRecord{SessionBaseRecord: sessionHistory.NextRecord("workflow_binding_ref"), BindingID: bindingID, WorkflowID: workflowHistory.WorkflowID, Action: "bind"})
		workflowHistory.Append(logs.WorkflowBindingRefRecord{WorkflowBaseRecord: workflowHistory.NextRecord("workflow_binding_ref"), BindingID: bindingID, AgentID: agentDef.Name, Action: "bind"})
	}
	if strings.TrimSpace(args.prompt) != "" {
		sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: args.prompt})
	}
	loop := runner.Loop{
		Provider:    providerAdapter,
		Tools:       toolRegistry,
		Projections: projections,
		MaxHistory:  maxHistory,
		StopToken:   args.stopToken,
	}
	var workflowDefPtr *defs.WorkflowDefinition
	if hasWorkflow {
		workflowDefPtr = &workflowDef
	}
	runErr := loop.Run(hydratedAgent, agentDef, workflowDefPtr, sessionHistory, workflowHistory)
	if statePath != "" {
		if err := logs.SaveState(statePath, sessionHistory, workflowHistory); err != nil {
			if runErr != nil {
				return fmt.Errorf("%v (also failed to save state: %w)", runErr, err)
			}
			return err
		}
	}
	if runErr != nil {
		return runErr
	}
	if args.showStats {
		printSessionStats(logs.ReduceSessionStats(sessionHistory), args)
		return nil
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
	sessionID    string
	statePath    string
	stopToken    string
	showStats    bool
	statsSection string
	statsTool    string
	statsState   string
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
		case "--session-id":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --session-id")
			}
			parsed.sessionID = args[i]
		case "--state":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --state")
			}
			parsed.statePath = args[i]
		case "--stop-token":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --stop-token")
			}
			parsed.stopToken = args[i]
		case "--stats":
			parsed.showStats = true
		case "--section":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --section")
			}
			parsed.statsSection = args[i]
		case "--tool":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --tool")
			}
			parsed.statsTool = args[i]
		case "--state-filter":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --state-filter")
			}
			parsed.statsState = args[i]
		default:
			return cliArgs{}, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if parsed.prompt == "" && parsed.statePath == "" && parsed.sessionID == "" {
		return cliArgs{}, fmt.Errorf("--prompt is required unless resuming with --state or --session-id")
	}
	return parsed, nil
}

func printSessionStats(stats logs.SessionStats, args cliArgs) {
	if strings.TrimSpace(args.statsTool) != "" {
		printToolStats(stats, args.statsTool)
		return
	}
	if strings.TrimSpace(args.statsState) != "" {
		printStateStats(stats, args.statsState)
		return
	}
	switch strings.TrimSpace(args.statsSection) {
	case "":
		printStatsSummary(stats)
	case "records":
		printRecordCounts(stats.RecordCounts)
	case "output":
		printOutputStats(stats.Output)
	case "tools":
		printToolSummary(stats.Tools, stats.ByTool)
	case "retry":
		printRetryStats(stats.Retry)
	case "completion":
		printCompletionStats(stats.Completion, stats.StopReasons)
	default:
		fmt.Printf("unknown stats section %q\n", args.statsSection)
	}
}

func printStatsSummary(stats logs.SessionStats) {
	fmt.Printf("session: %s\n", stats.SessionID)
	printRecordCounts(stats.RecordCounts)
	printOutputStats(stats.Output)
	printToolSummary(stats.Tools, stats.ByTool)
	printRetryStats(stats.Retry)
	printCompletionStats(stats.Completion, stats.StopReasons)
}

func printRecordCounts(counts logs.RecordCounts) {
	fmt.Println("records:")
	fmt.Printf("  total: %d\n", counts.Total)
	fmt.Printf("  users: %d\n", counts.Users)
	fmt.Printf("  assistants: %d\n", counts.Assistants)
	fmt.Printf("  tool_requests: %d\n", counts.ToolRequests)
	fmt.Printf("  tool_results: %d\n", counts.ToolResults)
	fmt.Printf("  output_evaluations: %d\n", counts.OutputEvaluations)
	fmt.Printf("  tool_validations: %d\n", counts.ToolValidations)
	fmt.Printf("  retries: %d\n", counts.Retries)
	fmt.Printf("  completions: %d\n", counts.Completions)
}

func printOutputStats(output logs.OutputStats) {
	fmt.Println("output:")
	fmt.Printf("  total: %d\n", output.Total)
	fmt.Printf("  valid: %d\n", output.Valid)
	fmt.Printf("  invalid: %d\n", output.Invalid)
	fmt.Printf("  missing_required: %d\n", output.MissingRequired)
	fmt.Printf("  wrong_state: %d\n", output.WrongState)
	fmt.Println("  validation_statuses:")
	for _, key := range logs.SortedMapKeys(output.ByValidationStatus) {
		fmt.Printf("    %s: %d\n", key, output.ByValidationStatus[key])
	}
	fmt.Println("  parse_statuses:")
	for _, key := range logs.SortedMapKeys(output.ByParseStatus) {
		fmt.Printf("    %s: %d\n", key, output.ByParseStatus[key])
	}
}

func printToolSummary(toolsStats logs.ToolStats, byTool map[string]logs.PerToolStats) {
	fmt.Println("tools:")
	fmt.Printf("  proposed: %d\n", toolsStats.Proposed)
	fmt.Printf("  valid_proposals: %d\n", toolsStats.ValidProposals)
	fmt.Printf("  invalid_proposals: %d\n", toolsStats.InvalidProposals)
	fmt.Printf("  executed: %d\n", toolsStats.Executed)
	fmt.Printf("  execution_success: %d\n", toolsStats.ExecutionSuccess)
	fmt.Printf("  execution_failures: %d\n", toolsStats.ExecutionFailures)
	fmt.Println("  invalid_reasons:")
	for _, key := range logs.SortedMapKeys(toolsStats.ByReason) {
		fmt.Printf("    %s: %d\n", key, toolsStats.ByReason[key])
	}
	fmt.Println("  by_tool:")
	for _, key := range logs.SortedMapKeys(byTool) {
		tool := byTool[key]
		fmt.Printf("    %s: proposed=%d valid=%d invalid=%d executed=%d success=%d failures=%d\n", key, tool.Proposed, tool.ValidProposals, tool.InvalidProposals, tool.Executed, tool.ExecutionSuccess, tool.ExecutionFailures)
	}
}

func printRetryStats(retry logs.RetryStats) {
	fmt.Println("retry:")
	fmt.Printf("  total: %d\n", retry.Total)
	fmt.Printf("  recovered: %d\n", retry.Recovered)
	fmt.Printf("  unrecovered: %d\n", retry.Unrecovered)
	fmt.Println("  reasons:")
	for _, key := range logs.SortedMapKeys(retry.ByReason) {
		fmt.Printf("    %s: %d\n", key, retry.ByReason[key])
	}
}

func printCompletionStats(completion logs.CompletionStats, stopReasons map[string]int) {
	fmt.Println("completion:")
	fmt.Printf("  total: %d\n", completion.Total)
	fmt.Printf("  completed: %d\n", completion.Completed)
	fmt.Printf("  incomplete: %d\n", completion.Incomplete)
	fmt.Printf("  latest_completed: %s\n", strconv.FormatBool(completion.LatestCompleted))
	fmt.Printf("  latest_stop_reason: %s\n", completion.LatestStopReason)
	fmt.Println("  stop_reasons:")
	for _, key := range logs.SortedMapKeys(stopReasons) {
		fmt.Printf("    %s: %d\n", key, stopReasons[key])
	}
}

func printToolStats(stats logs.SessionStats, toolName string) {
	tool, ok := stats.ByTool[toolName]
	if !ok {
		fmt.Printf("tool %q not found in session stats\n", toolName)
		return
	}
	fmt.Printf("tool: %s\n", toolName)
	fmt.Printf("  proposed: %d\n", tool.Proposed)
	fmt.Printf("  valid_proposals: %d\n", tool.ValidProposals)
	fmt.Printf("  invalid_proposals: %d\n", tool.InvalidProposals)
	fmt.Printf("  executed: %d\n", tool.Executed)
	fmt.Printf("  execution_success: %d\n", tool.ExecutionSuccess)
	fmt.Printf("  execution_failures: %d\n", tool.ExecutionFailures)
}

func printStateStats(stats logs.SessionStats, stateName string) {
	state, ok := stats.ByState[stateName]
	if !ok {
		fmt.Printf("state %q not found in session stats\n", stateName)
		return
	}
	fmt.Printf("state: %s\n", stateName)
	fmt.Printf("  output_evaluations: %d\n", state.OutputEvaluations)
	fmt.Printf("  valid: %d\n", state.Valid)
	fmt.Printf("  invalid: %d\n", state.Invalid)
	fmt.Printf("  missing_required: %d\n", state.MissingRequired)
	fmt.Printf("  wrong_state: %d\n", state.WrongState)
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
			"list_files",
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
				{Type: "system", Name: "system", Prompt: "You are maelstrom-code, a precise coding agent. Prefer high-signal discovery before editing. Use the narrowest tool that can answer the question. Validate changes after editing. This runtime may provide cognitive state and workflow state guidance. Treat those as operating constraints, not just background information. Each cognitive state has a precise purpose; once you have achieved that purpose, transition deliberately rather than lingering indefinitely. Each workflow state also has a precise purpose and may require prerequisites before planning or implementation. When workflow or cognitive guidance says planning is not ready, do not begin planning. When enough evidence has been gathered for the current state, summarize what you learned, ask clarifying questions if needed, or transition to the next appropriate state instead of continuing to explore by inertia. Avoid becoming meta about the workflow machinery itself unless the task is explicitly about that machinery."},
				{Type: "repo_context"},
				{Type: "binding"},
				{Type: "workflow_state"},
				{Type: "interaction"},
				{Type: "cognitive_state"},
				{Type: "messages"},
			},
		},
		Cognitive: defs.StatechartDefinition{
			InitialState: "observe",
			States: []defs.StateDefinition{
				{Name: "observe", Prompt: "Observe and gather evidence before acting.", VisibleTools: []string{"list_files", "search_files", "get_file_skeleton", "read_symbol", "find_references", "read_file"}, EnabledTools: []string{"list_files", "search_files", "get_file_skeleton", "read_symbol", "find_references", "read_file", "replace_text", "run_command"}},
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
		tools.ListFilesTool{RootDir: root},
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

func firstWorkflow(memory *catalog.Memory) (defs.WorkflowDefinition, bool) {
	for _, def := range memory.Workflows {
		return def, true
	}
	return defs.WorkflowDefinition{}, false
}

func hasBindingRecord(history *logs.SessionHistory) bool {
	if history == nil {
		return false
	}
	for _, record := range history.Records {
		binding, ok := record.(logs.SessionWorkflowBindingRecord)
		if ok && binding.Action == "bind" {
			return true
		}
		bindingPtr, ok := record.(*logs.SessionWorkflowBindingRecord)
		if ok && bindingPtr.Action == "bind" {
			return true
		}
	}
	return false
}

func describeRecord(record logs.SessionRecord) string {
	switch v := record.(type) {
	case logs.UserMessageRecord:
		return fmt.Sprintf("user: %s", v.Content)
	case *logs.UserMessageRecord:
		return fmt.Sprintf("user: %s", v.Content)
	case logs.AssistantMessageRecord:
		return fmt.Sprintf("assistant: %s", v.Content)
	case *logs.AssistantMessageRecord:
		return fmt.Sprintf("assistant: %s", v.Content)
	case logs.ToolCallRequestRecord:
		return fmt.Sprintf("tool request: %s %s", v.ToolName, v.Arguments)
	case *logs.ToolCallRequestRecord:
		return fmt.Sprintf("tool request: %s %s", v.ToolName, v.Arguments)
	case logs.ToolCallResultRecord:
		return fmt.Sprintf("tool result: %s\n%s", v.ToolName, v.Content)
	case *logs.ToolCallResultRecord:
		return fmt.Sprintf("tool result: %s\n%s", v.ToolName, v.Content)
	case logs.CognitiveTransitionRecord:
		return fmt.Sprintf("cognitive transition: %s -> %s via %s", v.FromState, v.ToState, v.Trigger)
	case *logs.CognitiveTransitionRecord:
		return fmt.Sprintf("cognitive transition: %s -> %s via %s", v.FromState, v.ToState, v.Trigger)
	case logs.SessionWorkflowBindingRecord:
		return fmt.Sprintf("binding: %s %s", v.Action, v.WorkflowID)
	case *logs.SessionWorkflowBindingRecord:
		return fmt.Sprintf("binding: %s %s", v.Action, v.WorkflowID)
	case logs.WorkflowTransitionRefRecord:
		return fmt.Sprintf("workflow transition: %s -> %s via %s", v.FromState, v.ToState, v.Trigger)
	case *logs.WorkflowTransitionRefRecord:
		return fmt.Sprintf("workflow transition: %s -> %s via %s", v.FromState, v.ToState, v.Trigger)
	case logs.InterruptRecord:
		return fmt.Sprintf("interrupt: %s", v.Reason)
	case *logs.InterruptRecord:
		return fmt.Sprintf("interrupt: %s", v.Reason)
	case logs.ResumeRecord:
		return fmt.Sprintf("resume: %s", v.Reason)
	case *logs.ResumeRecord:
		return fmt.Sprintf("resume: %s", v.Reason)
	case logs.ContextSnapshotRecord, *logs.ContextSnapshotRecord:
		return "context snapshot"
	case logs.InferenceEnvelopeRecord, *logs.InferenceEnvelopeRecord:
		return "inference envelope"
	default:
		return fmt.Sprintf("record: %T", record)
	}
}
