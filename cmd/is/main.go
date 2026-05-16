package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/comalice/inference_sketch/internal"
	"github.com/comalice/inference_sketch/internal/context"
	"github.com/comalice/inference_sketch/internal/providers"
	"github.com/comalice/inference_sketch/internal/session"
	"github.com/comalice/inference_sketch/internal/tools"
)

func main() {
	s := session.New()

	// ctxDef := context.MarshalFromYAML("filepath")
	c := context.NewFromDefinition(context.ContextDefinition{
		Model: "z-ai/glm-4.5-air:free",
		// Model: "openrouter/free",
		Tools: []internal.ToolDefinition{
			{
				Name:        "weather",
				Description: "Get the latest weather for a location.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"a": map[string]any{
							"type":        "string",
							"description": "location to query for",
						},
					},
					"required":             []string{"a"},
					"additionalProperties": false,
				},
			},
		},
	})

	driver := providers.NewOpenRouter(os.Getenv("OPENROUTER_API_KEY"))

	for {
		// var input string
		fmt.Print(">>> ")
		// fmt.Scanln(&input)

		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "reading stdin:", err)
		}

		s.Append(session.NewUserMessage(scanner.Text()))

		for {
			bundle, err := c.BuildInferenceBundle(s)
			if err != nil {
				panic(-1)
			}

			resp, err := driver.Send(&bundle)
			if err != nil {
				panic(-1)
			}

			respItems := resp.ToSessionItems()

			hasToolCalls := false

			// Iterate through response items, act on tool calls, append all to session.
			for _, item := range respItems {
				s.Append(item)

				fmt.Print(session.PrettyPrintItem(item))

				if tc, ok := item.(internal.ToolCallRequestMessage); ok {
					result := tools.Exec(tc)
					s.Append(result)
					hasToolCalls = true
				}
			}

			if !hasToolCalls {
				break // return to user, naive, need to inspect stop reason eventually
			}
		}
	}
}
