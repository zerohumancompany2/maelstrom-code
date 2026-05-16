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

	reg := tools.NewRegistry(tools.WeatherTool{})

	c := context.NewFromDefinition(context.ContextDefinition{
		Model: "z-ai/glm-4.5-air:free",
		Tools: reg.Definitions(),
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
					result := tools.Dispatch(reg, tc)
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
