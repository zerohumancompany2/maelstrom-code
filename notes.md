# Agent

- model
- context size
- temperature
- context definition

# context definition

- context chunks

# context chunks (interface)

- Build(a agent.Agent, s session.Session) []context.ContextItem <- defined by implementers of the interface

---

context map takes in a definition and then acts on the chunks iteratively to build the context

---

YAML -> ContextDefinition -> c ContextMap -> c.BuildInferenceBundle(session) -> InferenceBundle

---

- agent.BuildContextDefinition should use agents context specification
- contextItemContent << should probably move to getters/setters for stuff like this.