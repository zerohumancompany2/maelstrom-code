package evals

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/comalice/inference_sketch/sketch/sketch7/catalog"
	"github.com/comalice/inference_sketch/sketch/sketch7/compile"
	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/prompt"
	"github.com/comalice/inference_sketch/sketch/sketch7/provider"
	"github.com/comalice/inference_sketch/sketch/sketch7/runner"
	"github.com/comalice/inference_sketch/sketch/sketch7/statecharts"
	"github.com/comalice/inference_sketch/sketch/sketch7/tools"
)

type RunRecord struct {
	RunID       string            `json:"run_id"`
	DeckName    string            `json:"deck_name"`
	CaseID      string            `json:"case_id"`
	RepeatIndex int               `json:"repeat_index"`
	AgentID     string            `json:"agent_id"`
	ModelLabel  string            `json:"model_label,omitempty"`
	ModelID     string            `json:"model_id,omitempty"`
	ModelRef    string            `json:"model_ref,omitempty"`
	SessionID   string            `json:"session_id"`
	StartedAt   string            `json:"started_at"`
	FinishedAt  string            `json:"finished_at"`
	Passed      bool              `json:"passed"`
	Eval        EvalResult        `json:"eval"`
	Stats       logs.SessionStats `json:"stats"`
	Error       string            `json:"error,omitempty"`
}

type RunnerConfig struct {
	DeckPath         string
	OutputPath       string
	Provider         provider.Provider
	RootDir          string
	DefaultModelPath string
}

func RunDeck(config RunnerConfig) error {
	deck, err := LoadTaskDeck(config.DeckPath)
	if err != nil {
		return err
	}
	if config.Provider == nil {
		return fmt.Errorf("provider required")
	}
	if strings.TrimSpace(config.RootDir) == "" {
		config.RootDir = "."
	}
	if err := os.MkdirAll(filepath.Dir(config.OutputPath), 0o755); err != nil {
		return err
	}
	completed, err := completedRunIDs(config.OutputPath)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(config.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	for _, tc := range deck.Cases {
		for _, modelPath := range modelPathsFor(deck, tc, config.DefaultModelPath) {
			for repeat := 1; repeat <= tc.Repeats; repeat++ {
				runID := runIDFor(deck, tc, modelPath, repeat)
				if completed[runID] {
					continue
				}
				record := runCase(deck, tc, modelPath, repeat, config)
				if err := encoder.Encode(record); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// modelPathsFor resolves the model matrix for a case: an explicit case model
// wins, then the deck-level model list, then the CLI default model path.
func modelPathsFor(deck TaskDeck, tc TaskCase, defaultModelPath string) []string {
	if strings.TrimSpace(tc.ModelPath) != "" {
		return []string{tc.ModelPath}
	}
	paths := []string{}
	for _, path := range deck.Models {
		if strings.TrimSpace(path) != "" {
			paths = append(paths, path)
		}
	}
	if len(paths) > 0 {
		return paths
	}
	return []string{defaultModelPath}
}

// modelLabel derives a stable run ID component from a model file path. An
// empty path (CLI default fallback) labels as "default", so resumed batches
// assume the default model does not change between invocations.
func modelLabel(path string) string {
	base := strings.TrimSpace(filepath.Base(path))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if base == "" || base == "." {
		return "default"
	}
	return sanitizeID(base)
}

// completedRunIDs returns the run IDs already recorded in an existing JSONL
// batch file so interrupted batches can resume without duplicating work.
func completedRunIDs(path string) (map[string]bool, error) {
	completed := map[string]bool{}
	records, err := LoadRunRecords(path)
	if os.IsNotExist(err) {
		return completed, nil
	}
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		completed[record.RunID] = true
	}
	return completed, nil
}

func runIDFor(deck TaskDeck, tc TaskCase, modelPath string, repeat int) string {
	return fmt.Sprintf("%s:%s:%s:%d", sanitizeID(deck.Name), sanitizeID(tc.ID), modelLabel(modelPath), repeat)
}

func runCase(deck TaskDeck, tc TaskCase, modelPath string, repeat int, config RunnerConfig) RunRecord {
	started := time.Now().UTC()
	runID := runIDFor(deck, tc, modelPath, repeat)
	sessionID := fmt.Sprintf("eval-%s", runID)
	record := RunRecord{RunID: runID, DeckName: deck.Name, CaseID: tc.ID, RepeatIndex: repeat, ModelLabel: modelLabel(modelPath), SessionID: sessionID, StartedAt: started.Format(time.RFC3339Nano)}
	memory := catalog.NewMemory()
	for _, path := range []string{modelPath, tc.AgentPath, tc.WorkflowPath} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			record.Error = err.Error()
			record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
			return record
		}
		if err := catalog.LoadIntoMemory(memory, raw); err != nil {
			record.Error = err.Error()
			record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
			return record
		}
	}
	modelDef, ok := firstModel(memory)
	if !ok {
		record.Error = "no model definition loaded"
		record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return record
	}
	record.ModelID = modelDef.Name
	if len(modelDef.Providers) > 0 {
		record.ModelRef = modelDef.Providers[0].ModelRef
	}
	agentDef, ok := firstAgent(memory)
	if !ok {
		record.Error = "no agent definition loaded"
		record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return record
	}
	record.AgentID = agentDef.Name
	workflowDef, hasWorkflow := firstWorkflow(memory)
	var workflowDefPtr *defs.WorkflowDefinition
	if hasWorkflow {
		workflowDefPtr = &workflowDef
	}
	toolRegistry := buildEvalToolRegistry(config.RootDir, agentDef, workflowDefPtr)
	hydratedAgent, err := compile.HydrateAgent(agentDef, modelDef, toolRegistry)
	if err != nil {
		record.Error = err.Error()
		record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return record
	}
	projections, maxHistory, err := prompt.BuildProjectionPlan(agentDef)
	if err != nil {
		record.Error = err.Error()
		record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return record
	}
	loop := runner.Loop{Provider: config.Provider, Tools: toolRegistry, Projections: projections, MaxHistory: maxHistory, Deadline: started.Add(caseTimeout(tc))}
	sessionHistory := logs.NewSessionHistory(sessionID)
	sessionHistory.AgentID = agentDef.Name
	var workflowHistory *logs.WorkflowHistory
	if hasWorkflow {
		workflowHistory = logs.NewWorkflowHistory(sessionID + ":" + workflowDef.Name)
		bindingID := "bind-" + sessionID
		sessionHistory.Append(logs.SessionWorkflowBindingRecord{SessionBaseRecord: sessionHistory.NextRecord("workflow_binding_ref"), BindingID: bindingID, WorkflowID: workflowHistory.WorkflowID, Action: "bind"})
		workflowHistory.Append(logs.WorkflowBindingRefRecord{WorkflowBaseRecord: workflowHistory.NextRecord("workflow_binding_ref"), BindingID: bindingID, AgentID: agentDef.Name, Action: "bind"})
	}
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: tc.Prompt})
	if err := loop.Run(hydratedAgent, agentDef, workflowDefPtr, sessionHistory, workflowHistory); err != nil {
		record.Error = err.Error()
	}
	record.Stats = logs.ReduceSessionStats(sessionHistory)
	record.Eval = EvaluateSessionStats(record.Stats, tc.Eval)
	record.Passed = record.Error == "" && record.Eval.Passed
	record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return record
}

func buildEvalToolRegistry(root string, agentDef defs.AgentDefinition, workflowDef *defs.WorkflowDefinition) tools.Registry {
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
		tools.SearchFilesTool{RootDir: root},
		tools.GetFileSkeletonTool{RootDir: root},
		tools.FindReferencesTool{RootDir: root},
		tools.ReadSymbolTool{RootDir: root},
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

// defaultCaseTimeout bounds eval cases that set no explicit timeout so a
// misbehaving provider or runaway loop cannot stall a batch indefinitely.
const defaultCaseTimeout = 300 * time.Second

func caseTimeout(tc TaskCase) time.Duration {
	if tc.TimeoutSeconds > 0 {
		return time.Duration(tc.TimeoutSeconds) * time.Second
	}
	return defaultCaseTimeout
}

func sanitizeID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer(" ", "-", "/", "-", ":", "-", "_", "-")
	return replacer.Replace(value)
}
