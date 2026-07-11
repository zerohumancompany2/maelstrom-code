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
	StageID     string            `json:"stage_id,omitempty"`
	StageIndex  int               `json:"stage_index,omitempty"`
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
		if len(tc.Stages) > 0 {
			// Staged cases iterate models × repeats only (stage agents come
			// from the stages). The whole case is skipped iff ALL its stage
			// run IDs are already in completed; otherwise the whole case is
			// re-run but only records whose run IDs are NOT already in
			// completed are encoded (crash-window dedupe: a partial write
			// before a crash should not duplicate completed stage records).
			for _, modelPath := range modelPathsFor(deck, tc, config.DefaultModelPath) {
				for repeat := 1; repeat <= tc.Repeats; repeat++ {
					stageRunIDs := stageRunIDsFor(deck, tc, modelPath, repeat)
					allCompleted := true
					for _, id := range stageRunIDs {
						if !completed[id] {
							allCompleted = false
							break
						}
					}
					if allCompleted {
						continue
					}
					records := runStagedCase(deck, tc, modelPath, repeat, config)
					for _, record := range records {
						if completed[record.RunID] {
							continue
						}
						if err := encoder.Encode(record); err != nil {
							return err
						}
					}
				}
			}
			continue
		}
		for _, agentPath := range agentPathsFor(deck, tc) {
			for _, modelPath := range modelPathsFor(deck, tc, config.DefaultModelPath) {
				for repeat := 1; repeat <= tc.Repeats; repeat++ {
					runID := runIDFor(deck, tc, agentPath, modelPath, repeat)
					if completed[runID] {
						continue
					}
					record := runCase(deck, tc, agentPath, modelPath, repeat, config)
					if err := encoder.Encode(record); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// agentPathsFor resolves the agent matrix for a case: an explicit case agent
// wins, then the deck-level agent list.
func agentPathsFor(deck TaskDeck, tc TaskCase) []string {
	if strings.TrimSpace(tc.AgentPath) != "" {
		return []string{tc.AgentPath}
	}
	paths := []string{}
	for _, path := range deck.Agents {
		if strings.TrimSpace(path) != "" {
			paths = append(paths, path)
		}
	}
	if len(paths) > 0 {
		return paths
	}
	return []string{""}
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
	return pathLabel(path, "default")
}

// pathLabel derives a stable run ID component from a file path basename,
// falling back to the given label for empty paths.
func pathLabel(path, fallback string) string {
	base := strings.TrimSpace(filepath.Base(path))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if base == "" || base == "." {
		return fallback
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

func runIDFor(deck TaskDeck, tc TaskCase, agentPath, modelPath string, repeat int) string {
	return fmt.Sprintf("%s:%s:%s:%s:%d", sanitizeID(deck.Name), sanitizeID(tc.ID), pathLabel(agentPath, "agent"), modelLabel(modelPath), repeat)
}

func runCase(deck TaskDeck, tc TaskCase, agentPath, modelPath string, repeat int, config RunnerConfig) RunRecord {
	runID := runIDFor(deck, tc, agentPath, modelPath, repeat)
	sessionID := fmt.Sprintf("eval-%s", runID)
	record, _ := executeSession(deck, tc, runID, sessionID, tc.Prompt, agentPath, modelPath, tc.Eval, repeat, config, nil)
	return record
}

// executeSession runs one (agent, prompt) session against a workflow. When
// sharedWorkflow is nil a fresh WorkflowHistory is created (today's behavior);
// when non-nil it is reused so staged cases share one persisted workflow
// instance across stages. Bind records are appended on both the session and
// workflow sides with a bindingID unique per call. The caller owns unbind
// records for shared workflows (staged cases); executeSession never unbinds.
// The returned *logs.SessionHistory is non-nil when a workflow was bound,
// letting the caller append unbind records on the session side.
func executeSession(deck TaskDeck, tc TaskCase, runID, sessionID, promptText, agentPath, modelPath string, eval SessionEvalCase, repeat int, config RunnerConfig, sharedWorkflow *logs.WorkflowHistory) (RunRecord, *logs.SessionHistory) {
	started := time.Now().UTC()
	record := RunRecord{RunID: runID, DeckName: deck.Name, CaseID: tc.ID, RepeatIndex: repeat, ModelLabel: modelLabel(modelPath), SessionID: sessionID, StartedAt: started.Format(time.RFC3339Nano)}
	memory := catalog.NewMemory()
	for _, path := range []string{modelPath, agentPath, tc.WorkflowPath} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			record.Error = err.Error()
			record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
			return record, nil
		}
		if err := catalog.LoadIntoMemory(memory, raw); err != nil {
			record.Error = err.Error()
			record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
			return record, nil
		}
	}
	modelDef, ok := firstModel(memory)
	if !ok {
		record.Error = "no model definition loaded"
		record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return record, nil
	}
	record.ModelID = modelDef.Name
	if len(modelDef.Providers) > 0 {
		record.ModelRef = modelDef.Providers[0].ModelRef
	}
	agentDef, ok := firstAgent(memory)
	if !ok {
		record.Error = "no agent definition loaded"
		record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return record, nil
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
		return record, nil
	}
	projections, maxHistory, err := prompt.BuildProjectionPlan(agentDef)
	if err != nil {
		record.Error = err.Error()
		record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return record, nil
	}
	loop := runner.Loop{Provider: config.Provider, Tools: toolRegistry, Projections: projections, MaxHistory: maxHistory, Deadline: started.Add(caseTimeout(tc))}
	sessionHistory := logs.NewSessionHistory(sessionID)
	sessionHistory.AgentID = agentDef.Name
	var workflowHistory *logs.WorkflowHistory
	if hasWorkflow {
		if sharedWorkflow != nil {
			workflowHistory = sharedWorkflow
		} else {
			workflowHistory = logs.NewWorkflowHistory(sessionID + ":" + workflowDef.Name)
		}
		bindingID := "bind-" + sessionID
		sessionHistory.Append(logs.SessionWorkflowBindingRecord{SessionBaseRecord: sessionHistory.NextRecord("workflow_binding_ref"), BindingID: bindingID, WorkflowID: workflowHistory.WorkflowID, Action: "bind"})
		workflowHistory.Append(logs.WorkflowBindingRefRecord{WorkflowBaseRecord: workflowHistory.NextRecord("workflow_binding_ref"), BindingID: bindingID, AgentID: agentDef.Name, Action: "bind"})
	}
	sessionHistory.Append(logs.UserMessageRecord{SessionBaseRecord: sessionHistory.NextRecord("user"), Content: promptText})
	if err := loop.Run(hydratedAgent, agentDef, workflowDefPtr, sessionHistory, workflowHistory); err != nil {
		record.Error = err.Error()
	}
	record.Stats = logs.ReduceSessionStats(sessionHistory)
	record.Eval = EvaluateSessionStats(record.Stats, eval)
	record.Passed = record.Error == "" && record.Eval.Passed
	record.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	var sessionHist *logs.SessionHistory
	if hasWorkflow {
		sessionHist = sessionHistory
	}
	return record, sessionHist
}

// stageRunIDFor derives the run ID for one stage of a staged case.
func stageRunIDFor(deck TaskDeck, tc TaskCase, stage TaskStage, modelPath string, repeat, stageIndex int) string {
	return fmt.Sprintf("%s:%s:%s:%s:%d:stage%d-%s", sanitizeID(deck.Name), sanitizeID(tc.ID), pathLabel(stage.Agent, "agent"), modelLabel(modelPath), repeat, stageIndex, sanitizeID(stage.ID))
}

// stageRunIDsFor returns the ordered list of stage run IDs for one (model,
// repeat) of a staged case. Used by RunDeck to decide skip/dedupe.
func stageRunIDsFor(deck TaskDeck, tc TaskCase, modelPath string, repeat int) []string {
	ids := make([]string, 0, len(tc.Stages))
	for i, stage := range tc.Stages {
		ids = append(ids, stageRunIDFor(deck, tc, stage, effectiveModelPath(stage.Model, modelPath), repeat, i+1))
	}
	return ids
}

// effectiveModelPath returns the stage-level model override if set, else the
// case-level model path.
func effectiveModelPath(stageModel, caseModelPath string) string {
	if strings.TrimSpace(stageModel) != "" {
		return stageModel
	}
	return caseModelPath
}

// runStagedCase runs a staged case: one persisted WorkflowHistory shared
// across all stages. For each stage it calls executeSession with the shared
// workflow, then appends unbind records on both the session and workflow
// sides. If a stage errors, every remaining stage emits a "skipped: prior
// stage failed" record so every stage run ID is always present in the output
// and resume treats the case as complete.
func runStagedCase(deck TaskDeck, tc TaskCase, modelPath string, repeat int, config RunnerConfig) []RunRecord {
	records := make([]RunRecord, 0, len(tc.Stages))
	// Load the workflow definition once to derive the workflow ID for the
	// shared history. The definition is also reloaded inside executeSession
	// for hydration (the runtime needs it in memory).
	workflowID := tc.ID
	if raw, err := os.ReadFile(tc.WorkflowPath); err == nil {
		memory := catalog.NewMemory()
		if err := catalog.LoadIntoMemory(memory, raw); err == nil {
			if wf, ok := firstWorkflow(memory); ok {
				workflowID = wf.Name
			}
		}
	}
	sharedWorkflow := logs.NewWorkflowHistory(fmt.Sprintf("eval-%s:%s:%d:%s", sanitizeID(deck.Name), sanitizeID(tc.ID), repeat, workflowID))
	failed := false
	for i, stage := range tc.Stages {
		stageIndex := i + 1
		effectiveModel := effectiveModelPath(stage.Model, modelPath)
		runID := stageRunIDFor(deck, tc, stage, effectiveModel, repeat, stageIndex)
		sessionID := "eval-" + runID
		if failed {
			records = append(records, RunRecord{
				RunID:       runID,
				DeckName:    deck.Name,
				CaseID:      tc.ID,
				RepeatIndex: repeat,
				ModelLabel:  modelLabel(effectiveModel),
				SessionID:   sessionID,
				StageID:     stage.ID,
				StageIndex:  stageIndex,
				Passed:      false,
				Error:       "skipped: prior stage failed",
			})
			continue
		}
		record, stageSession := executeSession(deck, tc, runID, sessionID, stage.Prompt, stage.Agent, effectiveModel, stage.Eval, repeat, config, sharedWorkflow)
		record.StageID = stage.ID
		record.StageIndex = stageIndex
		// Unbind on both sides after the stage session ends (evals/ owns the
		// sequencing; the runtime never decides bind/unbind on its own). The
		// session-side unbind lands after stats reduction, which is fine:
		// binding records are bookkeeping, not evaluated behavior.
		if stageSession != nil {
			bindingID := "bind-" + sessionID
			stageSession.Append(logs.SessionWorkflowBindingRecord{
				SessionBaseRecord: stageSession.NextRecord("workflow_binding_ref"),
				BindingID:         bindingID,
				WorkflowID:        sharedWorkflow.WorkflowID,
				Action:            "unbind",
			})
			sharedWorkflow.Append(logs.WorkflowBindingRefRecord{
				WorkflowBaseRecord: sharedWorkflow.NextRecord("workflow_binding_ref"),
				BindingID:          bindingID,
				AgentID:            record.AgentID,
				Action:             "unbind",
			})
		}
		records = append(records, record)
		if record.Error != "" {
			failed = true
		}
	}
	return records
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
