package evals

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type BatchSummary struct {
	Total          int                    `json:"total"`
	Passed         int                    `json:"passed"`
	Failed         int                    `json:"failed"`
	Errored        int                    `json:"errored"`
	PassRate       float64                `json:"pass_rate"`
	InvalidOutputs int                    `json:"invalid_outputs"`
	StopReasons    map[string]int         `json:"stop_reasons"`
	FailedChecks   map[string]int         `json:"failed_checks"`
	ByCase         map[string]CaseSummary `json:"by_case"`
	ByAgent        map[string]CaseSummary `json:"by_agent"`
	ByModel        map[string]CaseSummary `json:"by_model"`
}

type CaseSummary struct {
	Total                int     `json:"total"`
	Passed               int     `json:"passed"`
	Errored              int     `json:"errored"`
	PassRate             float64 `json:"pass_rate"`
	InvalidOutputs       int     `json:"invalid_outputs"`
	UnrecoveredRetries   int     `json:"unrecovered_retries"`
	FinalizationFailures int     `json:"finalization_failures"`
}

// LoadRunRecords reads a JSONL batch file, keeping only the latest record per
// run ID so re-run batches remain idempotent for summaries.
func LoadRunRecords(path string) ([]RunRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	latest := map[string]RunRecord{}
	order := []string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var record RunRecord
		if err := json.Unmarshal([]byte(text), &record); err != nil {
			return nil, fmt.Errorf("parse run record line %d: %w", line, err)
		}
		if _, seen := latest[record.RunID]; !seen {
			order = append(order, record.RunID)
		}
		latest[record.RunID] = record
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	records := make([]RunRecord, 0, len(order))
	for _, runID := range order {
		records = append(records, latest[runID])
	}
	return records, nil
}

func SummarizeRuns(records []RunRecord) BatchSummary {
	summary := BatchSummary{
		StopReasons:  map[string]int{},
		FailedChecks: map[string]int{},
		ByCase:       map[string]CaseSummary{},
		ByAgent:      map[string]CaseSummary{},
		ByModel:      map[string]CaseSummary{},
	}
	for _, record := range records {
		summary.Total++
		if record.Passed {
			summary.Passed++
		} else {
			summary.Failed++
		}
		if strings.TrimSpace(record.Error) != "" {
			summary.Errored++
		}
		summary.InvalidOutputs += record.Stats.Output.Invalid
		for reason, count := range record.Stats.StopReasons {
			summary.StopReasons[reason] += count
		}
		for _, check := range record.Eval.Checks {
			if !check.Passed {
				summary.FailedChecks[check.Name]++
			}
		}
		summary.ByCase[record.CaseID] = accumulateCaseSummary(summary.ByCase[record.CaseID], record)
		if strings.TrimSpace(record.AgentID) != "" {
			summary.ByAgent[record.AgentID] = accumulateCaseSummary(summary.ByAgent[record.AgentID], record)
		}
		if key := modelSummaryKey(record); key != "" {
			summary.ByModel[key] = accumulateCaseSummary(summary.ByModel[key], record)
		}
	}
	summary.PassRate = passRate(summary.Passed, summary.Total)
	for key, value := range summary.ByCase {
		value.PassRate = passRate(value.Passed, value.Total)
		summary.ByCase[key] = value
	}
	for key, value := range summary.ByAgent {
		value.PassRate = passRate(value.Passed, value.Total)
		summary.ByAgent[key] = value
	}
	for key, value := range summary.ByModel {
		value.PassRate = passRate(value.Passed, value.Total)
		summary.ByModel[key] = value
	}
	return summary
}

// modelSummaryKey prefers the model definition name and falls back to the
// path-derived label for records that errored before loading a model.
func modelSummaryKey(record RunRecord) string {
	if strings.TrimSpace(record.ModelID) != "" {
		return record.ModelID
	}
	return strings.TrimSpace(record.ModelLabel)
}

func SummarizeBatchFile(path string) (BatchSummary, error) {
	records, err := LoadRunRecords(path)
	if err != nil {
		return BatchSummary{}, err
	}
	return SummarizeRuns(records), nil
}

func PrintBatchSummary(summary BatchSummary) {
	fmt.Printf("runs: %d\n", summary.Total)
	fmt.Printf("passed: %d\n", summary.Passed)
	fmt.Printf("failed: %d\n", summary.Failed)
	fmt.Printf("errored: %d\n", summary.Errored)
	fmt.Printf("pass_rate: %.2f\n", summary.PassRate)
	fmt.Printf("invalid_outputs: %d\n", summary.InvalidOutputs)
	printCountMap("stop_reasons", summary.StopReasons)
	printCountMap("failed_checks", summary.FailedChecks)
	printCaseSummaries("by_case", summary.ByCase)
	printCaseSummaries("by_agent", summary.ByAgent)
	printCaseSummaries("by_model", summary.ByModel)
}

func accumulateCaseSummary(current CaseSummary, record RunRecord) CaseSummary {
	current.Total++
	if record.Passed {
		current.Passed++
	}
	if strings.TrimSpace(record.Error) != "" {
		current.Errored++
	}
	current.InvalidOutputs += record.Stats.Output.Invalid
	current.UnrecoveredRetries += record.Stats.Retry.Unrecovered
	current.FinalizationFailures += record.Stats.StopReasons["finalization_validation_failed"]
	return current
}

func passRate(passed, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(passed) / float64(total)
}

func printCountMap(label string, values map[string]int) {
	fmt.Printf("%s:\n", label)
	if len(values) == 0 {
		fmt.Println("  none")
		return
	}
	for _, key := range sortedKeys(values) {
		fmt.Printf("  %s: %d\n", key, values[key])
	}
}

func printCaseSummaries(label string, values map[string]CaseSummary) {
	fmt.Printf("%s:\n", label)
	if len(values) == 0 {
		fmt.Println("  none")
		return
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := values[key]
		fmt.Printf("  %s: total=%d passed=%d errored=%d pass_rate=%.2f invalid=%d unrecovered_retries=%d finalization_failures=%d\n",
			key, value.Total, value.Passed, value.Errored, value.PassRate, value.InvalidOutputs, value.UnrecoveredRetries, value.FinalizationFailures)
	}
}

func sortedKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
