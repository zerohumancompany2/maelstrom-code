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
	"github.com/comalice/inference_sketch/internal/yaml"
)

func main() {
	s := session.New()

	agent, err := yaml.UnmarshalYAMLToAgent(`id: weather-assistant-v1
name: Weather Assistant
version: "1.0.0"
description: Reliable weather lookup

model:
  name: "z-ai/glm-4.5-air:free"
  provider: "openrouter"
  contextLength: 32768
  temperature: 0.7

systemPrompt: |
  You are a cheerful, accurate weather assistant.
  Always respond in Celsius. Be concise and friendly.`)
	if err != nil {
		panic(err)
	}

	reg := tools.NewRegistry(tools.WeatherTool{})

	c := context.NewFromDefinition(agent.BuildContextDefinition(reg))

	driver := providers.NewOpenRouter(os.Getenv("OPENROUTER_API_KEY"))

	for {
		fmt.Print(">>> ")

		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "reading stdin:", err)
		}

		s.Append(session.NewUserMessage(scanner.Text()))

		for {
			messages, err := c.BuildMessages(s)
			if err != nil {
				fmt.Fprintf(os.Stderr, "BuildInferenceBundle error: %v\n", err)
				panic(-1)
			}

			resp, err := driver.Send(messages, providers.ProviderOptions{
				Model: c.Model,
				Tools: c.Tools,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Provider error: %v\n", err)
				panic(-1)
			}

			respItems := resp.ToSessionItems()

			hasToolCalls := false

			// Iterate through response items, act on tool calls, append all to session.
			for _, item := range respItems {
				s.Append(item)

				fmt.Println(session.PrettyPrintItem(item))

				if tc, ok := item.(internal.ToolCallRequestMessage); ok {
					result := tools.Dispatch(reg, tc)
					s.Append(result)
					fmt.Println(session.PrettyPrintItem(result))
					hasToolCalls = true
				}
			}

			if !hasToolCalls {
				break // return to user, naive, need to inspect stop reason eventually
			}
		}
	}
}
