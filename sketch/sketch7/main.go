package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch7/catalog"
	"github.com/comalice/inference_sketch/sketch/sketch7/compile"
	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/evals"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runner"
	"github.com/comalice/inference_sketch/sketch/sketch7/statecharts"
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
	sessionDir := filepath.Join(".maelstrom", "sessions")
	store := logs.FileSessionStore{SessionDir: sessionDir}
	if args.aggregateByAgent {
		report, err := logs.AggregateSessionStatsByAgentStore(store, sessionDir)
		if err != nil {
			return err
		}
		printAggregateByAgent(report, args)
		return nil
	}
	if strings.TrimSpace(args.evalSummaryPath) != "" {
		summary, err := evals.SummarizeBatchFile(args.evalSummaryPath)
		if err != nil {
			return err
		}
		if strings.EqualFold(strings.TrimSpace(args.statsFormat), "json") {
			raw, err := json.MarshalIndent(summary, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(raw))
			return nil
		}
		evals.PrintBatchSummary(summary)
		return nil
	}
	if strings.TrimSpace(args.evalDeckPath) != "" {
		providerAdapter, err := providerFromEnv()
		if err != nil {
			return err
		}
		outputPath := strings.TrimSpace(args.evalOutputPath)
		if outputPath == "" {
			outputPath = filepath.Join(".maelstrom", "evals", "latest.jsonl")
		}
		return evals.RunDeck(evals.RunnerConfig{DeckPath: args.evalDeckPath, OutputPath: outputPath, Provider: providerAdapter, RootDir: ".", DefaultModelPath: args.modelPath})
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
	statePath := strings.TrimSpace(args.statePath)
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
	} else if strings.TrimSpace(args.sessionID) != "" {
		loadedSession, loadedWorkflow, err := store.LoadSession(strings.TrimSpace(args.sessionID))
		if err == nil {
			sessionHistory = loadedSession
			workflowHistory = loadedWorkflow
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if sessionHistory == nil {
		sessionID := strings.TrimSpace(args.sessionID)
		if sessionID == "" {
			sessionID = "session-001"
		}
		sessionHistory = logs.NewSessionHistory(sessionID)
	}
	if (args.showStats || args.showReport) && strings.TrimSpace(args.prompt) == "" {
		stats := logs.ReduceSessionStats(sessionHistory)
		if args.showReport {
			printSessionReport(stats, args)
			return nil
		}
		printSessionStats(stats, args)
		return nil
	}
	modelDef, ok := firstModel(memory)
	if !ok {
		modelDef = defaultModelDefinition()
	}
	agentDef, ok := firstAgent(memory)
	if !ok {
		agentDef = defaultAgentDefinition(modelDef.Name)
	}
	if strings.TrimSpace(sessionHistory.AgentID) == "" {
		sessionHistory.AgentID = agentDef.Name
	}
	workflowDef, hasWorkflow := firstWorkflow(memory)
	var workflowDefPtr *defs.WorkflowDefinition
	if hasWorkflow {
		workflowDefPtr = &workflowDef
	}
	toolRegistry := buildToolRegistry(agentDef, workflowDefPtr)
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
	runErr := loop.Run(hydratedAgent, agentDef, workflowDefPtr, sessionHistory, workflowHistory)
	if statePath != "" {
		if err := logs.SaveState(statePath, sessionHistory, workflowHistory); err != nil {
			if runErr != nil {
				return fmt.Errorf("%v (also failed to save state: %w)", runErr, err)
			}
			return err
		}
	} else if strings.TrimSpace(args.sessionID) != "" {
		if err := store.SaveSession(strings.TrimSpace(args.sessionID), sessionHistory, workflowHistory); err != nil {
			if runErr != nil {
				return fmt.Errorf("%v (also failed to save session: %w)", runErr, err)
			}
			return err
		}
	}
	if runErr != nil {
		return runErr
	}
	if args.showReport {
		printSessionReport(logs.ReduceSessionStats(sessionHistory), args)
		return nil
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
	modelPath        string
	agentPath        string
	workflowPath     string
	prompt           string
	sessionID        string
	statePath        string
	stopToken        string
	showStats        bool
	showReport       bool
	aggregateByAgent bool
	statsSection     string
	statsTool        string
	statsState       string
	statsFormat      string
	evalDeckPath     string
	evalOutputPath   string
	evalSummaryPath  string
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
		case "--report":
			parsed.showReport = true
		case "--aggregate-by-agent":
			parsed.aggregateByAgent = true
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
		case "--format":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --format")
			}
			parsed.statsFormat = args[i]
		case "--eval-deck":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --eval-deck")
			}
			parsed.evalDeckPath = args[i]
		case "--eval-out":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --eval-out")
			}
			parsed.evalOutputPath = args[i]
		case "--eval-summary":
			i++
			if i >= len(args) {
				return cliArgs{}, fmt.Errorf("missing value for --eval-summary")
			}
			parsed.evalSummaryPath = args[i]
		default:
			return cliArgs{}, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if strings.TrimSpace(parsed.evalDeckPath) != "" || strings.TrimSpace(parsed.evalSummaryPath) != "" {
		return parsed, nil
	}
	if parsed.prompt == "" && parsed.statePath == "" && parsed.sessionID == "" {
		if parsed.aggregateByAgent {
			return parsed, nil
		}
		return cliArgs{}, fmt.Errorf("--prompt is required unless resuming with --state or --session-id")
	}
	return parsed, nil
}

func printSessionStats(stats logs.SessionStats, args cliArgs) {
	if strings.EqualFold(strings.TrimSpace(args.statsFormat), "json") {
		printSessionStatsJSON(stats, args)
		return
	}
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

func printSessionStatsJSON(stats logs.SessionStats, args cliArgs) {
	var value any = stats
	if strings.TrimSpace(args.statsTool) != "" {
		tool, ok := stats.ByTool[args.statsTool]
		if !ok {
			value = map[string]any{"tool": args.statsTool, "error": "not found"}
		} else {
			value = map[string]any{"tool": args.statsTool, "stats": tool}
		}
	} else if strings.TrimSpace(args.statsState) != "" {
		state, ok := stats.ByState[args.statsState]
		if !ok {
			value = map[string]any{"state": args.statsState, "error": "not found"}
		} else {
			value = map[string]any{"state": args.statsState, "stats": state}
		}
	} else {
		switch strings.TrimSpace(args.statsSection) {
		case "":
			value = stats
		case "records":
			value = stats.RecordCounts
		case "output":
			value = stats.Output
		case "tools":
			value = map[string]any{"summary": stats.Tools, "by_tool": stats.ByTool}
		case "retry":
			value = stats.Retry
		case "completion":
			value = map[string]any{"summary": stats.Completion, "stop_reasons": stats.StopReasons}
		default:
			value = map[string]any{"section": args.statsSection, "error": "unknown section"}
		}
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Printf("failed to marshal stats json: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func printSessionReport(stats logs.SessionStats, args cliArgs) {
	if strings.EqualFold(strings.TrimSpace(args.statsFormat), "json") {
		printSessionReportJSON(stats)
		return
	}
	fmt.Printf("report: %s\n", stats.SessionID)
	fmt.Printf("completion: %s\n", reportCompletionLine(stats.Completion))
	fmt.Printf("output: total=%d valid=%d invalid=%d missing_required=%d wrong_state=%d\n", stats.Output.Total, stats.Output.Valid, stats.Output.Invalid, stats.Output.MissingRequired, stats.Output.WrongState)
	fmt.Printf("tools: proposed=%d valid=%d invalid=%d executed=%d success=%d failures=%d\n", stats.Tools.Proposed, stats.Tools.ValidProposals, stats.Tools.InvalidProposals, stats.Tools.Executed, stats.Tools.ExecutionSuccess, stats.Tools.ExecutionFailures)
	fmt.Printf("retry: total=%d recovered=%d unrecovered=%d\n", stats.Retry.Total, stats.Retry.Recovered, stats.Retry.Unrecovered)
	printReportAttribution(stats)
	fmt.Println("dominant failure modes:")
	for _, failure := range dominantFailureModes(stats) {
		fmt.Printf("  - [%s] %s (layer: %s)\n", failure.Severity, failure.Summary, failure.Layer)
	}
	fmt.Println("recommended changes:")
	for _, recommendation := range recommendedChanges(stats) {
		fmt.Printf("  - [%s] %s\n", recommendation.Severity, recommendation.Change)
		fmt.Printf("    layer: %s\n", recommendation.Layer)
		fmt.Printf("    files: %s\n", strings.Join(recommendation.Files, ", "))
	}
}

func printSessionReportJSON(stats logs.SessionStats) {
	value := map[string]any{
		"session_id":             stats.SessionID,
		"completion":             reportCompletionLine(stats.Completion),
		"attribution":            reportAttribution(stats),
		"dominant_failure_modes": dominantFailureModes(stats),
		"recommended_changes":    recommendedChanges(stats),
		"stats":                  stats,
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Printf("failed to marshal report json: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func reportCompletionLine(completion logs.CompletionStats) string {
	if completion.LatestCompleted {
		return fmt.Sprintf("completed (%s)", completion.LatestStopReason)
	}
	if strings.TrimSpace(completion.LatestStopReason) == "" {
		return "incomplete"
	}
	return fmt.Sprintf("incomplete (%s)", completion.LatestStopReason)
}

type ReportFailure struct {
	Severity string `json:"severity"`
	Layer    string `json:"layer"`
	Summary  string `json:"summary"`
	Target   string `json:"target,omitempty"`
}

type ReportRecommendation struct {
	Severity string   `json:"severity"`
	Layer    string   `json:"layer"`
	Change   string   `json:"change"`
	Files    []string `json:"files"`
	Target   string   `json:"target,omitempty"`
}

type countEntry struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

func dominantFailureModes(stats logs.SessionStats) []ReportFailure {
	modes := []ReportFailure{}
	if stats.Output.Invalid > 0 {
		modes = append(modes, ReportFailure{Severity: "high", Layer: "output_contract", Summary: fmt.Sprintf("invalid output contract evaluations: %d", stats.Output.Invalid), Target: topKey(stats.Output.ByValidationStatus)})
	}
	if bucketFailureCount(stats.Output.ByValidationStatus) > 0 {
		modes = append(modes, ReportFailure{Severity: "high", Layer: "output_buckets", Summary: fmt.Sprintf("bucket contract failures: %d", bucketFailureCount(stats.Output.ByValidationStatus)), Target: topBucketFailure(stats.Output.ByValidationStatus)})
	}
	if stats.Output.MissingRequired > 0 {
		modes = append(modes, ReportFailure{Severity: "high", Layer: "output_contract", Summary: fmt.Sprintf("missing required output fields: %d", stats.Output.MissingRequired), Target: topKey(stats.Output.MissingFieldCounts)})
	}
	if stats.Output.WrongState > 0 {
		modes = append(modes, ReportFailure{Severity: "high", Layer: "state_projection", Summary: fmt.Sprintf("wrong-state emissions: %d", stats.Output.WrongState), Target: topFailingState(stats.ByState)})
	}
	if stats.Tools.InvalidProposals > 0 {
		modes = append(modes, ReportFailure{Severity: "medium", Layer: "tool_validation", Summary: fmt.Sprintf("invalid tool proposals: %d", stats.Tools.InvalidProposals), Target: topInvalidTool(stats.ByTool)})
	}
	if stats.Tools.ExecutionFailures > 0 {
		modes = append(modes, ReportFailure{Severity: "medium", Layer: "tool_execution", Summary: fmt.Sprintf("tool execution failures: %d", stats.Tools.ExecutionFailures), Target: topExecutionFailureTool(stats.ByTool)})
	}
	if stats.Retry.Unrecovered > 0 {
		modes = append(modes, ReportFailure{Severity: "medium", Layer: "retry", Summary: fmt.Sprintf("unrecovered retries: %d", stats.Retry.Unrecovered), Target: topKey(stats.Retry.Attribution.ByCauseKind)})
	}
	if boundStopCount(stats.StopReasons) > 0 {
		modes = append(modes, ReportFailure{Severity: "medium", Layer: "state_bounds", Summary: fmt.Sprintf("bound-triggered stops: %d", boundStopCount(stats.StopReasons)), Target: topBoundStop(stats.StopReasons)})
	}
	if len(modes) == 0 {
		modes = append(modes, ReportFailure{Severity: "info", Layer: "session", Summary: "no dominant failure modes detected in current session"})
	}
	return modes
}

func recommendedChanges(stats logs.SessionStats) []ReportRecommendation {
	changes := []ReportRecommendation{}
	if stats.Output.Invalid > 0 || stats.Output.MissingRequired > 0 {
		changes = append(changes, ReportRecommendation{Severity: "high", Layer: "output_contract", Change: "tighten state output projection text and simplify required output fields in state contracts", Files: []string{"sketch/sketch7/prompt/projection.go", "sketch/sketch7/runner/loop.go", "sketch/sketch7/defs/workflow.go"}, Target: topKey(stats.Output.BySchema)})
	}
	if bucketFailureCount(stats.Output.ByValidationStatus) > 0 {
		changes = append(changes, ReportRecommendation{Severity: "high", Layer: "output_buckets", Change: "clarify finalization-mode bucket requirements and keep wrapped cognitive/workflow outputs explicit in task framing", Files: []string{"sketch/sketch7/runner/output_eval.go", "sketch/sketch7/context/sections.go", "docs/planning/sketch7-core-completion-plan.md"}, Target: topBucketFailure(stats.Output.ByValidationStatus)})
	}
	if stats.Output.WrongState > 0 {
		changes = append(changes, ReportRecommendation{Severity: "high", Layer: "state_projection", Change: "improve state-local prompt projection and validate state field more explicitly at the loop boundary", Files: []string{"sketch/sketch7/prompt/projection.go", "sketch/sketch7/runtime/reduce.go", "sketch/sketch7/runner/loop.go"}, Target: topFailingState(stats.ByState)})
	}
	if stats.Tools.InvalidProposals > 0 {
		changes = append(changes, ReportRecommendation{Severity: "medium", Layer: "tool_validation", Change: "tighten tool argument validation feedback and simplify ambiguous tool interfaces", Files: []string{"sketch/sketch7/runner/loop.go", "sketch/sketch7/tools/*.go", "sketch/sketch7/provider/provider.go"}, Target: topInvalidTool(stats.ByTool)})
	}
	if stats.Tools.ExecutionFailures > 0 {
		changes = append(changes, ReportRecommendation{Severity: "medium", Layer: "tool_execution", Change: "improve tool execution error surfacing and exact-match tool semantics", Files: []string{"sketch/sketch7/tools/*.go", "sketch/sketch7/tools/*_test.go"}, Target: topExecutionFailureTool(stats.ByTool)})
	}
	if stats.Retry.Unrecovered > 0 {
		changes = append(changes, ReportRecommendation{Severity: "medium", Layer: "retry", Change: "improve retry nudges and preserve structured validation errors in retry feedback", Files: []string{"sketch/sketch7/runner/loop.go", "sketch/sketch7/provider/openai_compatible.go"}, Target: topKey(stats.Retry.Attribution.ByCauseKind)})
	}
	if !stats.Completion.LatestCompleted {
		changes = append(changes, ReportRecommendation{Severity: "medium", Layer: "loop_completion", Change: "tighten completion signaling and loop stop conditions", Files: []string{"sketch/sketch7/runner/loop.go", "sketch/sketch7/main.go", "sketch/sketch7/runner/loop_test.go"}, Target: stats.Completion.LatestStopReason})
	}
	if boundStopCount(stats.StopReasons) > 0 {
		changes = append(changes, ReportRecommendation{Severity: "medium", Layer: "state_bounds", Change: "review state bounds and finalization thresholds so bounded exits are intentional rather than accidental", Files: []string{"sketch/sketch7/defs/workflow.go", "sketch/sketch7/runtime/reduce.go", "sketch/sketch7/runner/loop.go"}, Target: topBoundStop(stats.StopReasons)})
	}
	if len(changes) == 0 {
		changes = append(changes, ReportRecommendation{Severity: "info", Layer: "eval", Change: "current session looks healthy; next step is to validate against broader eval tasks", Files: []string{"docs/evals.md", "docs/planning/state-scoped-io-contracts-rollout.md"}})
	}
	return changes
}

func printReportAttribution(stats logs.SessionStats) {
	fmt.Println("attribution:")
	printTopCountMap("  validation_statuses", stats.Output.ByValidationStatus)
	printTopCountMap("  bucket_validation_statuses", bucketValidationStatuses(stats.Output.ByValidationStatus))
	printTopCountMap("  parse_statuses", stats.Output.ByParseStatus)
	printTopCountMap("  wrapped_parse_statuses", wrappedParseStatuses(stats.Output.ByParseStatus))
	printTopCountMap("  schemas", stats.Output.BySchema)
	printTopCountMap("  missing_output_fields", stats.Output.MissingFieldCounts)
	printTopCountMap("  tool_invalid_reasons", stats.Tools.ByReason)
	printTopCountMap("  retry_causes", stats.Retry.Attribution.ByCauseKind)
	printTopCountMap("  retry_tools", stats.Retry.Attribution.ByTool)
	printTopCountMap("  models", stats.ModelRefs)
	printTopCountMap("  providers", stats.ProviderRefs)
	printTopCountMap("  state_exit_reasons", stats.StateExitReasons)
	printTopCountMap("  bound_stop_reasons", boundStopReasons(stats.StopReasons))
	if stats.Completion.LatestCognitiveState != "" || stats.Completion.LatestWorkflowState != "" {
		fmt.Printf("  terminal_state: cognitive=%s workflow=%s iteration=%d\n", stats.Completion.LatestCognitiveState, stats.Completion.LatestWorkflowState, stats.Completion.LatestIteration)
	}
}

func printTopCountMap(label string, values map[string]int) {
	fmt.Printf("%s:\n", label)
	entries := topCountEntries(values, 5)
	if len(entries) == 0 {
		fmt.Println("    none")
		return
	}
	for _, entry := range entries {
		fmt.Printf("    %s: %d\n", entry.Key, entry.Count)
	}
}

func reportAttribution(stats logs.SessionStats) map[string]any {
	return map[string]any{
		"validation_statuses":        topCountEntries(stats.Output.ByValidationStatus, 5),
		"bucket_validation_statuses": topCountEntries(bucketValidationStatuses(stats.Output.ByValidationStatus), 5),
		"parse_statuses":             topCountEntries(stats.Output.ByParseStatus, 5),
		"wrapped_parse_statuses":     topCountEntries(wrappedParseStatuses(stats.Output.ByParseStatus), 5),
		"schemas":                    topCountEntries(stats.Output.BySchema, 5),
		"missing_output_fields":      topCountEntries(stats.Output.MissingFieldCounts, 5),
		"tool_invalid_reasons":       topCountEntries(stats.Tools.ByReason, 5),
		"retry_causes":               topCountEntries(stats.Retry.Attribution.ByCauseKind, 5),
		"retry_tools":                topCountEntries(stats.Retry.Attribution.ByTool, 5),
		"models":                     topCountEntries(stats.ModelRefs, 5),
		"providers":                  topCountEntries(stats.ProviderRefs, 5),
		"state_exit_reasons":         topCountEntries(stats.StateExitReasons, 5),
		"bound_stop_reasons":         topCountEntries(boundStopReasons(stats.StopReasons), 5),
		"terminal_cognitive_state":   stats.Completion.LatestCognitiveState,
		"terminal_workflow_state":    stats.Completion.LatestWorkflowState,
		"terminal_iteration":         stats.Completion.LatestIteration,
	}
}

func bucketValidationStatuses(values map[string]int) map[string]int {
	filtered := map[string]int{}
	for key, count := range values {
		switch key {
		case "missing_required_buckets", "missing_cognitive_bucket", "missing_workflow_bucket", "invalid_cognitive_bucket", "invalid_workflow_bucket", "unknown_bucket":
			filtered[key] = count
		}
	}
	return filtered
}

func wrappedParseStatuses(values map[string]int) map[string]int {
	filtered := map[string]int{}
	for key, count := range values {
		if key == "valid_json_wrapped" {
			filtered[key] = count
		}
	}
	return filtered
}

func boundStopReasons(values map[string]int) map[string]int {
	filtered := map[string]int{}
	for key, count := range values {
		switch key {
		case "max_tool_calls", "max_wall_time", "max_inference_turns":
			filtered[key] = count
		}
	}
	return filtered
}

func bucketFailureCount(values map[string]int) int {
	total := 0
	for _, count := range bucketValidationStatuses(values) {
		total += count
	}
	return total
}

func boundStopCount(values map[string]int) int {
	total := 0
	for _, count := range boundStopReasons(values) {
		total += count
	}
	return total
}

func topBucketFailure(values map[string]int) string {
	return topKey(bucketValidationStatuses(values))
}

func topBoundStop(values map[string]int) string {
	return topKey(boundStopReasons(values))
}

func topCountEntries(values map[string]int, limit int) []countEntry {
	entries := make([]countEntry, 0, len(values))
	for key, count := range values {
		if key == "" || count == 0 {
			continue
		}
		entries = append(entries, countEntry{Key: key, Count: count})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Count == entries[j].Count {
			return entries[i].Key < entries[j].Key
		}
		return entries[i].Count > entries[j].Count
	})
	if limit > 0 && len(entries) > limit {
		return entries[:limit]
	}
	return entries
}

func topKey(values map[string]int) string {
	entries := topCountEntries(values, 1)
	if len(entries) == 0 {
		return ""
	}
	return entries[0].Key
}

func topFailingState(values map[string]logs.PerStateStats) string {
	best := ""
	bestScore := 0
	for key, state := range values {
		score := state.Invalid + state.MissingRequired + state.WrongState
		if score > bestScore || (score == bestScore && score > 0 && (best == "" || key < best)) {
			best = key
			bestScore = score
		}
	}
	return best
}

func topInvalidTool(values map[string]logs.PerToolStats) string {
	best := ""
	bestScore := 0
	for key, tool := range values {
		score := tool.InvalidProposals
		if score > bestScore || (score == bestScore && score > 0 && (best == "" || key < best)) {
			best = key
			bestScore = score
		}
	}
	return best
}

func topExecutionFailureTool(values map[string]logs.PerToolStats) string {
	best := ""
	bestScore := 0
	for key, tool := range values {
		score := tool.ExecutionFailures
		if score > bestScore || (score == bestScore && score > 0 && (best == "" || key < best)) {
			best = key
			bestScore = score
		}
	}
	return best
}

func printAggregateByAgent(report logs.SessionAggregateReport, args cliArgs) {
	if strings.EqualFold(strings.TrimSpace(args.statsFormat), "json") {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Printf("failed to marshal aggregate json: %v\n", err)
			return
		}
		fmt.Println(string(data))
		return
	}
	fmt.Printf("aggregate-by-agent: %s\n", report.SessionDir)
	for _, agentID := range logs.SortedMapKeys(report.Agents) {
		agg := report.Agents[agentID]
		fmt.Printf("agent: %s\n", agentID)
		fmt.Printf("  sessions: %d\n", agg.SessionCount)
		fmt.Printf("  completion_rate: %.2f\n", agg.CompletionRate)
		fmt.Printf("  invalid_rate: %.2f\n", agg.InvalidRate)
		fmt.Printf("  tool_failure_rate: %.2f\n", agg.ToolFailureRate)
		fmt.Printf("  retry_failure_rate: %.2f\n", agg.RetryFailureRate)
		fmt.Printf("  session_ids: %s\n", strings.Join(agg.SessionIDs, ", "))
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
				{Type: "system", Name: "system", Prompt: "You are maelstrom-code, a precise coding agent. Prefer high-signal discovery before editing. Use the narrowest tool that can answer the question. Validate changes after editing. This runtime may provide a current task frame with task-local inputs, allowed tools, output requirements, and completion constraints. Treat that task frame as operating guidance. Do not become meta about orchestration machinery unless the user explicitly asks about it."},
				{Type: "repo_context"},
				{Type: "binding"},
				{Type: "state_task"},
				{Type: "interaction"},
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

func buildToolRegistry(agentDef defs.AgentDefinition, workflowDef *defs.WorkflowDefinition) tools.Registry {
	root, _ := os.Getwd()
	transitionTool := tools.TransitionTool{AgentChart: statecharts.Compile("agent", agentDef.Cognitive)}
	if workflowDef != nil {
		transitionTool.WorkflowChart = statecharts.Compile("workflow", workflowDef.Statechart)
	}
	return tools.NewRegistry(
		tools.BindingTool{},
		tools.UnbindTool{},
		tools.InterruptTool{},
		tools.ResumeTool{},
		transitionTool,
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
