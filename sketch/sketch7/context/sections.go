package context

import (
	"fmt"
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
		case "state_task":
			content := formatStateTask(session, history)
			sections = append(sections, Section{Name: "state_task", Role: "system", Content: content, Sticky: true, LogicalKey: "state_task", SectionType: projection.Type, SourceKind: "derived_state_task", RetentionMode: projection.RetentionMode})
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

func formatStateTask(session runtime.SessionView, history *logs.SessionHistory) string {
	parts := []string{}

	// Current task prompt from cognitive state
	if strings.TrimSpace(session.Cognitive.Prompt) != "" {
		parts = append(parts, fmt.Sprintf("Current task: %s", strings.TrimSpace(session.Cognitive.Prompt)))
	}

	// Available tools
	effectiveTools := effectiveEnabledTools(session)
	if len(effectiveTools) > 0 {
		parts = append(parts, fmt.Sprintf("Available tools: %s", joinList(effectiveTools)))
	}
	if len(session.Cognitive.AllowedTriggers) > 0 {
		parts = append(parts, fmt.Sprintf("Allowed next-step signals: %s", joinList(session.Cognitive.AllowedTriggers)))
	}

	// Output requirements if any
	if strings.TrimSpace(session.Cognitive.Outputs.SchemaName) != "" || len(session.Cognitive.Outputs.RequiredFields) > 0 {
		parts = append(parts, fmt.Sprintf("Required output schema: %s. Required fields: %s.", strings.TrimSpace(session.Cognitive.Outputs.SchemaName), joinList(session.Cognitive.Outputs.RequiredFields)))
	}
	if mode := runtime.ResolveFinalizationMode(session.Cognitive, session.Workflow, history); mode.IsFinalizing {
		buckets := []string{}
		if mode.RequireCognitive {
			buckets = append(buckets, `"cognitive"`)
		}
		if mode.RequireWorkflow {
			buckets = append(buckets, `"workflow"`)
		}
		parts = append(parts, fmt.Sprintf("Finalization mode: tool use is closed for this task. Return a single JSON object whose top-level key(s) are %s, each holding its required output schema fields. Finalization attempt: %d.", strings.Join(buckets, " and "), mode.RetryAttempt))
	}
	if bounds := formatBounds("Task bounds", session.Cognitive.Bounds); bounds != "" {
		parts = append(parts, bounds)
	}

	// Workflow context if bound
	if session.Workflow != nil {
		wf := session.Workflow
		if strings.TrimSpace(wf.Description) != "" {
			parts = append(parts, fmt.Sprintf("Task context: %s", wf.Description))
		}
		if strings.TrimSpace(wf.Context) != "" {
			parts = append(parts, fmt.Sprintf("Additional context: %s", wf.Context))
		}
		if strings.TrimSpace(wf.Outputs.SchemaName) != "" || len(wf.Outputs.RequiredFields) > 0 {
			parts = append(parts, fmt.Sprintf("Workflow output schema: %s. Required fields: %s.", strings.TrimSpace(wf.Outputs.SchemaName), joinList(wf.Outputs.RequiredFields)))
		}
		if len(wf.Inputs.Required) > 0 {
			parts = append(parts, fmt.Sprintf("Workflow required inputs: %s", joinList(wf.Inputs.Required)))
		}
		if len(wf.Inputs.Optional) > 0 {
			parts = append(parts, fmt.Sprintf("Workflow optional inputs: %s", joinList(wf.Inputs.Optional)))
		}
		if len(wf.Completion.SuccessWhen) > 0 {
			parts = append(parts, fmt.Sprintf("Workflow completion conditions: %s", joinList(wf.Completion.SuccessWhen)))
		}
		if bounds := formatBounds("Workflow bounds", wf.Bounds); bounds != "" {
			parts = append(parts, bounds)
		}
	}

	// Input expectations from cognitive state
	if len(session.Cognitive.Inputs.Required) > 0 {
		parts = append(parts, fmt.Sprintf("Required inputs: %s", joinList(session.Cognitive.Inputs.Required)))
	}
	if len(session.Cognitive.Inputs.Optional) > 0 {
		parts = append(parts, fmt.Sprintf("Optional inputs: %s", joinList(session.Cognitive.Inputs.Optional)))
	}

	return strings.Join(parts, " ")
}

func formatBounds(label string, bounds defs.StateBoundsContract) string {
	items := []string{}
	if bounds.MaxInferenceTurns > 0 {
		items = append(items, fmt.Sprintf("max inference turns=%d", bounds.MaxInferenceTurns))
	}
	if bounds.MaxToolCalls > 0 {
		items = append(items, fmt.Sprintf("max tool calls=%d", bounds.MaxToolCalls))
	}
	if bounds.MaxWallTimeSeconds > 0 {
		items = append(items, fmt.Sprintf("max wall time seconds=%d", bounds.MaxWallTimeSeconds))
	}
	if bounds.MaxFinalizationRetries > 0 {
		items = append(items, fmt.Sprintf("max finalization retries=%d", bounds.MaxFinalizationRetries))
	}
	if len(items) == 0 {
		return ""
	}
	return fmt.Sprintf("%s: %s.", label, strings.Join(items, "; "))
}

func effectiveEnabledTools(session runtime.SessionView) []string {
	allowed := append([]string(nil), session.Agent.ToolNames...)
	policyApplied := false
	if len(session.Cognitive.EnabledTools) > 0 || len(session.Cognitive.VisibleTools) > 0 {
		policyApplied = true
		if len(allowed) == 0 {
			allowed = append([]string(nil), session.Cognitive.EnabledTools...)
		} else {
			allowed = intersectPreservingOrder(allowed, session.Cognitive.EnabledTools)
		}
	}
	if session.Workflow != nil && (len(session.Workflow.EnabledTools) > 0 || len(session.Workflow.VisibleTools) > 0) {
		policyApplied = true
		if len(allowed) == 0 {
			allowed = append([]string(nil), session.Workflow.EnabledTools...)
		} else {
			allowed = intersectPreservingOrder(allowed, session.Workflow.EnabledTools)
		}
	}
	if len(allowed) == 0 && !policyApplied {
		return append([]string(nil), session.Agent.ToolNames...)
	}
	return allowed
}

func intersectPreservingOrder(base []string, filter []string) []string {
	result := make([]string, 0)
	for _, item := range base {
		if containsString(filter, item) {
			result = append(result, item)
		}
	}
	return result
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
