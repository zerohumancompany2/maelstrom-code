package context

import (
	"fmt"
	"sort"
	"strings"

	"github.com/comalice/inference_sketch/sketch/sketch7/defs"
	"github.com/comalice/inference_sketch/sketch/sketch7/logs"
	"github.com/comalice/inference_sketch/sketch/sketch7/runtime"
)

var defaultRepoCache RepoContextCache

func BuildSections(agentDef defs.AgentDefinition, session runtime.SessionView, history *logs.SessionHistory, opts RepoContextOptions) []Section {
	sections := []Section{}
	for _, projection := range agentDef.Context.Projections {
		switch projection.Type {
		case "system":
			sections = append(sections, Section{Name: projection.Name, Role: "system", Content: projection.Prompt, Sticky: true, LogicalKey: projection.Name, SectionType: projection.Type, SourceKind: "static", RetentionMode: projection.RetentionMode})
		case "interaction":
			content := fmt.Sprintf("Session interaction mode: %s.", session.Interaction.Mode)
			if strings.TrimSpace(session.Interaction.Reason) != "" {
				content = fmt.Sprintf("Session interaction mode: %s. Reason: %s.", session.Interaction.Mode, session.Interaction.Reason)
			}
			sections = append(sections, Section{Name: "interaction", Role: "system", Content: content, Sticky: true, LogicalKey: "interaction", SectionType: projection.Type, SourceKind: "derived_interaction", RetentionMode: projection.RetentionMode})
		case "cognitive_state":
			sections = append(sections, Section{Name: "cognitive", Role: "system", Content: fmt.Sprintf("Cognitive mode: %s. Prompt: %s. Visible tools: %s. Enabled tools: %s.", session.Cognitive.CurrentState, strings.TrimSpace(session.Cognitive.Prompt), joinList(session.Cognitive.VisibleTools), joinList(session.Cognitive.EnabledTools)), Sticky: true, LogicalKey: "cognitive_state", SectionType: projection.Type, SourceKind: "derived_cognitive", RetentionMode: projection.RetentionMode})
		case "workflow_state":
			if session.Workflow != nil {
				content := formatWorkflowSection(session.Workflow, history)
				sections = append(sections, Section{Name: "workflow", Role: "system", Content: content, Sticky: true, LogicalKey: "workflow_state", SectionType: projection.Type, SourceKind: "derived_workflow", RetentionMode: projection.RetentionMode})
			}
		case "binding":
			if session.Binding.Bound {
				sections = append(sections, Section{Name: "binding", Role: "system", Content: fmt.Sprintf("Active binding: workflow=%s.", session.Binding.WorkflowID), Sticky: true, LogicalKey: "binding", SectionType: projection.Type, SourceKind: "derived_binding", RetentionMode: projection.RetentionMode})
			}
		case "repo_context":
			if projection.RefreshEveryNTurns != nil {
				opts.RefreshEveryTurns = *projection.RefreshEveryNTurns
			}
			summary := defaultRepoCache.Summary(opts, history)
			if strings.TrimSpace(summary) != "" {
				sections = append(sections, Section{Name: "repo_context", Role: "system", Content: summary, Sticky: true, LogicalKey: "repo_context", SectionType: projection.Type, SourceKind: "derived_repo", RefreshEveryTurns: opts.RefreshEveryTurns, RetentionMode: projection.RetentionMode})
			}
		}
	}
	return sections
}

func formatWorkflowSection(workflow *runtime.WorkflowView, history *logs.SessionHistory) string {
	if workflow == nil {
		return ""
	}
	completed, remaining, planningReady := workflowProgress(workflow, history)
	parts := []string{
		fmt.Sprintf("Workflow directive: you are currently in workflow state %s for workflow %s.", workflow.CurrentState, workflow.WorkflowID),
		fmt.Sprintf("Workflow description: %s.", workflow.Description),
		fmt.Sprintf("Workflow context: %s.", workflow.Context),
		fmt.Sprintf("Workflow-visible tools for this state: %s. Workflow-enabled tools for this state: %s.", joinList(workflow.VisibleTools), joinList(workflow.EnabledTools)),
	}
	if len(completed) > 0 {
		parts = append(parts, fmt.Sprintf("Completed checkpoints: %s.", strings.Join(completed, ", ")))
	}
	if len(remaining) > 0 {
		parts = append(parts, fmt.Sprintf("Remaining before planning: %s.", strings.Join(remaining, ", ")))
	} else {
		parts = append(parts, "Remaining before planning: none.")
	}
	if planningReady {
		parts = append(parts, "Planning readiness: ready. You may move into planning when appropriate.")
	} else {
		parts = append(parts, "Planning readiness: not ready. Do not begin planning yet.")
	}
	parts = append(parts, workflowObjective(workflow.CurrentState))
	if len(remaining) > 0 {
		parts = append(parts, fmt.Sprintf("Before planning, you must still complete: %s.", strings.Join(remaining, ", ")))
	}
	nextTransitions := suggestedNextTransitions(workflow.CurrentState, remaining, planningReady)
	if len(nextTransitions) > 0 {
		parts = append(parts, fmt.Sprintf("Suggested next transitions from this state: %s.", strings.Join(nextTransitions, ", ")))
	}
	return strings.Join(parts, " ")
}

func workflowProgress(workflow *runtime.WorkflowView, history *logs.SessionHistory) ([]string, []string, bool) {
	required := []string{"repo_orientation", "clarification"}
	visited := map[string]bool{}
	if workflow.CurrentState != "" {
		visited[workflow.CurrentState] = true
	}
	if history != nil {
		for _, record := range history.Records {
			transition, ok := record.(logs.WorkflowTransitionRefRecord)
			if ok {
				visited[transition.ToState] = true
				continue
			}
			transitionPtr, ok := record.(*logs.WorkflowTransitionRefRecord)
			if ok {
				visited[transitionPtr.ToState] = true
			}
		}
	}
	completed := []string{}
	for _, state := range []string{"intake", "repo_orientation", "clarification", "planning", "plan_review", "implementation", "validation"} {
		if visited[state] {
			completed = append(completed, state)
		}
	}
	remaining := []string{}
	for _, state := range required {
		if !visited[state] {
			remaining = append(remaining, state)
		}
	}
	sort.Strings(remaining)
	return completed, remaining, len(remaining) == 0
}

func workflowObjective(state string) string {
	switch state {
	case "intake":
		return "Immediate objective: decide whether repo orientation or clarification is the best next step for this task."
	case "repo_orientation":
		return "Immediate objective: inspect the repository and gather evidence relevant to the task without drifting into planning prematurely."
	case "clarification":
		return "Immediate objective: ask focused follow-up questions informed by the task and any repo evidence already gathered."
	case "planning":
		return "Immediate objective: produce a concrete, reviewable plan grounded in requirements and repo evidence."
	case "plan_review":
		return "Immediate objective: present the plan clearly and wait for approval, correction, or redirection."
	case "implementation":
		return "Immediate objective: execute the approved plan with narrow, auditable repo changes."
	case "validation":
		return "Immediate objective: validate the implementation and determine whether work is complete or needs revision."
	default:
		return "Immediate objective: follow the workflow state deliberately before moving onward."
	}
}

func suggestedNextTransitions(state string, remaining []string, planningReady bool) []string {
	suggestions := []string{}
	switch state {
	case "intake":
		suggestions = append(suggestions, "begin_repo_orientation", "begin_clarification")
	case "repo_orientation":
		if containsString(remaining, "clarification") {
			suggestions = append(suggestions, "begin_clarification")
		}
		if planningReady {
			suggestions = append(suggestions, "start_planning")
		}
		if len(suggestions) == 0 {
			suggestions = append(suggestions, "gather_more_context")
		}
	case "clarification":
		if containsString(remaining, "repo_orientation") {
			suggestions = append(suggestions, "begin_repo_orientation")
		}
		if planningReady {
			suggestions = append(suggestions, "start_planning")
		}
		if len(suggestions) == 0 {
			suggestions = append(suggestions, "continue_clarification")
		}
	case "planning":
		suggestions = append(suggestions, "submit_plan_for_review")
	case "plan_review":
		suggestions = append(suggestions, "approve_plan", "revise_plan")
	case "implementation":
		suggestions = append(suggestions, "begin_validation", "continue_implementation")
	case "validation":
		suggestions = append(suggestions, "complete", "fix_implementation")
	}
	return suggestions
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
