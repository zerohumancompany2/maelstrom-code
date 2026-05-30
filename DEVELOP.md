# Error Surfacing

- runtime concerns like tool call failures should not use golang's internal errors, but rather surface the errors through the available types and/or as log output
  Example: internal/providers/openrouter.go:serializeArguments; error is returned as the output so the model and the user can see it