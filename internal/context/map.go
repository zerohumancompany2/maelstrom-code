package context

import (
	"fmt"
	"sort"

	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/session"
)

type ContextMap struct {
	Model        string // placeholder until we get proper definitions
	Tools        []internal.ToolDefinition
	Definition   ContextDefinition
	ContextLimit int
	Tokenizer    Tokenizer
}

type builtChunk struct {
	index int
	spec  ChunkSpec
	items []ContextItem
}

func (c *ContextMap) BuildMessages(s session.Session) ([]ContextItem, error) {
	var messages []ContextItem

	if len(c.Definition.Chunks) == 0 {
		return messages, nil
	}
	if c.ContextLimit <= 0 {
		return nil, fmt.Errorf("context limit must be greater than zero")
	}

	tokenizer := c.Tokenizer
	if tokenizer == nil {
		tokenizer = &HeuristicTokenizer{}
	}

	built := make([]builtChunk, 0, len(c.Definition.Chunks))
	fixedBudget := 0

	for idx, spec := range c.Definition.Chunks {
		items, err := spec.Chunk.Build(s)
		if err != nil {
			return nil, err
		}
		built = append(built, builtChunk{index: idx, spec: spec, items: items})
		if !spec.Flexible {
			fixedBudget += chunkBudget(spec, c.ContextLimit)
		}
	}

	remainingBudget := c.ContextLimit - fixedBudget
	if remainingBudget < 0 {
		remainingBudget = 0
	}

	for i := range built {
		spec := built[i].spec
		budget := chunkBudget(spec, c.ContextLimit)
		if spec.Flexible {
			budget = remainingBudget
		}
		policy := spec.Policy
		if policy == nil {
			policy = &HardTruncate{}
		}
		trimmed, err := policy.Trim(built[i].items, budget, tokenizer)
		if err != nil {
			return nil, err
		}
		built[i].items = trimmed
	}

	total := totalTokens(tokenizer, built)
	if total > c.ContextLimit {
		sort.SliceStable(built, func(i, j int) bool {
			return built[i].spec.Priority < built[j].spec.Priority
		})
		for i := range built {
			if total <= c.ContextLimit {
				break
			}
			policy := built[i].spec.Policy
			if policy == nil {
				policy = &HardTruncate{}
			}
			availableBudget := c.ContextLimit - (total - tokenizer.CountTokens(built[i].items...))
			if availableBudget < 0 {
				availableBudget = 0
			}
			trimmed, err := policy.Trim(built[i].items, availableBudget, tokenizer)
			if err != nil {
				continue
			}
			built[i].items = trimmed
			total = totalTokens(tokenizer, built)
		}
		if total > c.ContextLimit {
			return nil, fmt.Errorf("context exceeds limit after rebalance")
		}
		sort.SliceStable(built, func(i, j int) bool {
			return built[i].index < built[j].index
		})
	}

	for _, chunk := range built {
		messages = append(messages, chunk.items...)
	}

	return messages, nil
}

func chunkBudget(spec ChunkSpec, contextLimit int) int {
	if contextLimit <= 0 || spec.BudgetPct <= 0 {
		return 0
	}
	return int(spec.BudgetPct * float64(contextLimit))
}

func totalTokens(tokenizer Tokenizer, built []builtChunk) int {
	total := 0
	for _, chunk := range built {
		total += tokenizer.CountTokens(chunk.items...)
	}
	return total
}
