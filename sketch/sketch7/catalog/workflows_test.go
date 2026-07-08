package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
	"github.com/comalice/inference_sketch/sketch/sketch7/statecharts"
)

// readOnlyEvalTools mirrors the read-only subset of the eval tool registry
// (evals.buildEvalToolRegistry). Repository workflows are read-only until
// Phase 4 write gating lands, so every tool they reference must be here.
var readOnlyEvalTools = map[string]bool{
	"list_files":        true,
	"search_files":      true,
	"get_file_skeleton": true,
	"read_symbol":       true,
	"find_references":   true,
	"read_file":         true,
}

// loadRepositoryWorkflows loads all workflow YAML files from the ../workflows directory.
func loadRepositoryWorkflows(t *testing.T) *Memory {
	t.Helper()
	memory := NewMemory()
	pattern := filepath.Join("..", "workflows", "*.yaml")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("failed to glob workflows: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no workflow files found matching %s", pattern)
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read workflow file %s: %v", file, err)
		}
		if err := LoadIntoMemory(memory, raw); err != nil {
			t.Fatalf("failed to load workflow %s: %v", file, err)
		}
	}
	return memory
}

func TestRepositoryAgentsLoad(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "agents", "*.yaml"))
	if err != nil {
		t.Fatalf("failed to glob agents: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no agent files found")
	}
	memory := NewMemory()
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read agent file %s: %v", file, err)
		}
		if err := LoadIntoMemory(memory, raw); err != nil {
			t.Fatalf("failed to load agent %s: %v", file, err)
		}
	}
	if _, ok := memory.GetAgent("workflow-reader"); !ok {
		t.Fatal("expected workflow-reader agent in memory")
	}
}

func TestRepositoryWorkflowIssueTriageLoads(t *testing.T) {
	memory := loadRepositoryWorkflows(t)
	def, ok := memory.GetWorkflow("issue-triage")
	if !ok {
		t.Fatal("expected issue-triage workflow to exist")
	}
	if def.Statechart.InitialState != "triaging" {
		t.Fatalf("InitialState = %q, want triaging", def.Statechart.InitialState)
	}
	// Find the triaging state
	var triagingState *defs.StateDefinition
	for i := range def.Statechart.States {
		if def.Statechart.States[i].Name == "triaging" {
			triagingState = &def.Statechart.States[i]
			break
		}
	}
	if triagingState == nil {
		t.Fatal("triaging state not found")
	}
	// Check outputs schema
	if triagingState.Outputs.SchemaName != "triage_v1" {
		t.Fatalf("Outputs.SchemaName = %q, want triage_v1", triagingState.Outputs.SchemaName)
	}
	assertStringSlice(t, "triaging requiredFields", triagingState.Outputs.RequiredFields, []string{"decision", "evidence", "suspect_files", "transition"})
	assertStringSlice(t, "triaging allowedTriggers", triagingState.AllowedTriggers, []string{"finish"})
	// Check bounds
	if triagingState.Bounds.MaxInferenceTurns != 6 {
		t.Fatalf("MaxInferenceTurns = %d, want 6", triagingState.Bounds.MaxInferenceTurns)
	}
	if triagingState.Bounds.MaxToolCalls != 10 {
		t.Fatalf("MaxToolCalls = %d, want 10", triagingState.Bounds.MaxToolCalls)
	}
	if triagingState.Bounds.MaxFinalizationRetries != 2 {
		t.Fatalf("MaxFinalizationRetries = %d, want 2", triagingState.Bounds.MaxFinalizationRetries)
	}
	// Check exactly one transition: finish triaging->done
	if len(def.Statechart.Transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(def.Statechart.Transitions))
	}
	trans := def.Statechart.Transitions[0]
	if trans.Trigger != "finish" || trans.From != "triaging" || trans.To != "done" {
		t.Fatalf("transition = %+v, want finish: triaging->done", trans)
	}
}

func TestRepositoryWorkflowChangePlanningLoads(t *testing.T) {
	memory := loadRepositoryWorkflows(t)
	def, ok := memory.GetWorkflow("change-planning")
	if !ok {
		t.Fatal("expected change-planning workflow to exist")
	}
	if len(def.Statechart.States) != 5 {
		t.Fatalf("got %d states, want 5", len(def.Statechart.States))
	}
	if len(def.Statechart.Transitions) != 4 {
		t.Fatalf("got %d transitions, want 4", len(def.Statechart.Transitions))
	}
	// Compile the statechart and walk the chain
	machine := statecharts.Compile("workflow", def.Statechart)
	// intake --triaged--> inspecting
	next, err := machine.Next("intake", "triaged")
	if err != nil {
		t.Fatalf("Next(intake, triaged) error: %v", err)
	}
	if next != "inspecting" {
		t.Fatalf("Next(intake, triaged) = %q, want inspecting", next)
	}
	// inspecting --inspected--> planning
	next, err = machine.Next("inspecting", "inspected")
	if err != nil {
		t.Fatalf("Next(inspecting, inspected) error: %v", err)
	}
	if next != "planning" {
		t.Fatalf("Next(inspecting, inspected) = %q, want planning", next)
	}
	// planning --planned--> reviewing
	next, err = machine.Next("planning", "planned")
	if err != nil {
		t.Fatalf("Next(planning, planned) error: %v", err)
	}
	if next != "reviewing" {
		t.Fatalf("Next(planning, planned) = %q, want reviewing", next)
	}
	// reviewing --reviewed--> done
	next, err = machine.Next("reviewing", "reviewed")
	if err != nil {
		t.Fatalf("Next(reviewing, reviewed) error: %v", err)
	}
	if next != "done" {
		t.Fatalf("Next(reviewing, reviewed) = %q, want done", next)
	}
	// Exact per-stage contracts. Each stage's required inputs name the previous
	// stage's artifact fields — the handoff chain the Tier 2 harness relies on.
	stages := []struct {
		name            string
		schema          string
		requiredFields  []string
		allowedTriggers []string
		requiredInputs  []string
	}{
		{"intake", "intake_v1", []string{"request_summary", "unknowns", "transition"}, []string{"triaged"}, []string{"user_request"}},
		{"inspecting", "inspection_v1", []string{"relevant_files", "findings", "transition"}, []string{"inspected"}, []string{"request_summary", "unknowns"}},
		{"planning", "plan_v1", []string{"steps", "risks", "validation", "transition"}, []string{"planned"}, []string{"relevant_files", "findings"}},
		{"reviewing", "review_v1", []string{"verdict", "objections", "transition"}, []string{"reviewed"}, []string{"steps", "risks", "validation"}},
	}
	for _, want := range stages {
		state := findState(t, def, want.name)
		if state.Outputs.SchemaName != want.schema {
			t.Fatalf("state %q schema = %q, want %q", want.name, state.Outputs.SchemaName, want.schema)
		}
		assertStringSlice(t, want.name+" requiredFields", state.Outputs.RequiredFields, want.requiredFields)
		assertStringSlice(t, want.name+" allowedTriggers", state.AllowedTriggers, want.allowedTriggers)
		assertStringSlice(t, want.name+" inputs.required", state.Inputs.Required, want.requiredInputs)
		if state.Bounds.MaxToolCalls <= 0 {
			t.Fatalf("state %q MaxToolCalls = %d, want > 0", want.name, state.Bounds.MaxToolCalls)
		}
		if state.Bounds.MaxInferenceTurns <= 0 {
			t.Fatalf("state %q MaxInferenceTurns = %d, want > 0", want.name, state.Bounds.MaxInferenceTurns)
		}
		if state.Bounds.MaxFinalizationRetries <= 0 {
			t.Fatalf("state %q MaxFinalizationRetries = %d, want > 0", want.name, state.Bounds.MaxFinalizationRetries)
		}
	}
}

func findState(t *testing.T, def defs.WorkflowDefinition, name string) defs.StateDefinition {
	t.Helper()
	for _, state := range def.Statechart.States {
		if state.Name == name {
			return state
		}
	}
	t.Fatalf("state %q not found in workflow %q", name, def.Name)
	return defs.StateDefinition{}
}

func assertStringSlice(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] = %q, want %q", label, i, got[i], want[i])
		}
	}
}

func TestRepositoryWorkflowsAreCoherent(t *testing.T) {
	memory := loadRepositoryWorkflows(t)
	// Iterate over all workflows in memory
	for name, def := range memory.Workflows {
		// initialState is a declared state
		stateNames := make(map[string]bool)
		for _, state := range def.Statechart.States {
			stateNames[state.Name] = true
		}
		if !stateNames[def.Statechart.InitialState] {
			t.Fatalf("workflow %q: initialState %q is not a declared state", name, def.Statechart.InitialState)
		}
		// Every transition's From and To are declared states
		for _, trans := range def.Statechart.Transitions {
			if !stateNames[trans.From] {
				t.Fatalf("workflow %q: transition trigger %q From %q is not a declared state", name, trans.Trigger, trans.From)
			}
			if !stateNames[trans.To] {
				t.Fatalf("workflow %q: transition trigger %q To %q is not a declared state", name, trans.Trigger, trans.To)
			}
		}
		// Build transition map: from -> trigger -> to
		transMap := make(map[string]map[string]string)
		for _, trans := range def.Statechart.Transitions {
			if _, ok := transMap[trans.From]; !ok {
				transMap[trans.From] = make(map[string]string)
			}
			transMap[trans.From][trans.Trigger] = trans.To
		}
		// Every state's every allowedTrigger has a matching transition with From == that state
		for _, state := range def.Statechart.States {
			for _, trigger := range state.AllowedTriggers {
				if transFrom, ok := transMap[state.Name]; !ok {
					t.Fatalf("workflow %q: state %q allowedTrigger %q has no matching transition", name, state.Name, trigger)
				} else if _, ok := transFrom[trigger]; !ok {
					t.Fatalf("workflow %q: state %q allowedTrigger %q has no matching transition", name, state.Name, trigger)
				}
			}
			// Every state that has allowedTriggers also declares a non-empty outputs schema
			if len(state.AllowedTriggers) > 0 {
				if state.Outputs.SchemaName == "" {
					t.Fatalf("workflow %q: state %q has allowedTriggers but empty outputs schema", name, state.Name)
				}
			}
			// Every state's enabledTools is a subset of its visibleTools
			visibleSet := make(map[string]bool)
			for _, tool := range state.VisibleTools {
				visibleSet[tool] = true
			}
			for _, tool := range state.EnabledTools {
				if !visibleSet[tool] {
					t.Fatalf("workflow %q: state %q enabledTool %q is not in visibleTools", name, state.Name, tool)
				}
			}
			// Tool names must be real registry tools: a typo'd name would
			// silently narrow the runtime tool intersection to nothing.
			// Read-only set only — write tools wait for Phase 4 gating.
			for _, tool := range append(append([]string(nil), state.VisibleTools...), state.EnabledTools...) {
				if !readOnlyEvalTools[tool] {
					t.Fatalf("workflow %q: state %q references unknown or non-read-only tool %q", name, state.Name, tool)
				}
			}
		}
	}
}

func TestRepositoryWorkflowHydratesInitialView(t *testing.T) {
	memory := loadRepositoryWorkflows(t)
	def, ok := memory.GetWorkflow("issue-triage")
	if !ok {
		t.Fatal("expected issue-triage workflow to exist")
	}
	history := logs.NewWorkflowHistory("wf-hydrate-test")
	view := runtime.ReduceWorkflowState(history, def, "agent-x")
	if view.CurrentState != "triaging" {
		t.Fatalf("CurrentState = %q, want triaging", view.CurrentState)
	}
	// Check AllowedTriggers contains "finish"
	found := false
	for _, trigger := range view.AllowedTriggers {
		if trigger == "finish" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("AllowedTriggers = %v, want to contain finish", view.AllowedTriggers)
	}
	if view.Outputs.SchemaName != "triage_v1" {
		t.Fatalf("Outputs.SchemaName = %q, want triage_v1", view.Outputs.SchemaName)
	}
	if len(view.EnabledTools) == 0 {
		t.Fatal("EnabledTools is empty, want non-empty")
	}
	if view.Bounds.MaxToolCalls != 10 {
		t.Fatalf("Bounds.MaxToolCalls = %d, want 10", view.Bounds.MaxToolCalls)
	}
}
