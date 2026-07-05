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
	f, err := os.OpenFile(config.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	for _, tc := range deck.Cases {
		for repeat := 1; repeat <= tc.Repeats; repeat++ {
			record := runCase(deck, tc, repeat, config)
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
	}
	return nil
}

func runCase(deck TaskDeck, tc TaskCase, repeat int, config RunnerConfig) RunRecord {
	started := time.Now().UTC()
	runID := fmt.Sprintf("%s:%s:%d", sanitizeID(deck.Name), sanitizeID(tc.ID), repeat)
	sessionID := fmt.Sprintf("eval-%s", runID)
	record := RunRecord{RunID: runID, DeckName: deck.Name, CaseID: tc.ID, RepeatIndex: repeat, SessionID: sessionID, StartedAt: started.Format(time.RFC3339Nano)}
	memory := catalog.NewMemory()
	for _, path := range []string{choosePath(tc.ModelPath, config.DefaultModelPath), tc.AgentPath, tc.WorkflowPath} {
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
	loop := runner.Loop{Provider: config.Provider, Tools: toolRegistry, Projections: projections, MaxHistory: maxHistory}
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

func choosePath(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func sanitizeID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer(" ", "-", "/", "-", ":", "-", "_", "-")
	return replacer.Replace(value)
}
