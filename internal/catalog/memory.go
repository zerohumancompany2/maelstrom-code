package catalog

import "github.com/comalice/maelstrom/internal/defs"

type Memory struct {
	Models    map[string]defs.ModelDefinition
	Agents    map[string]defs.AgentDefinition
	Workflows map[string]defs.WorkflowDefinition
}

func NewMemory() *Memory {
	return &Memory{
		Models:    map[string]defs.ModelDefinition{},
		Agents:    map[string]defs.AgentDefinition{},
		Workflows: map[string]defs.WorkflowDefinition{},
	}
}

func (m *Memory) PutModel(def defs.ModelDefinition) {
	m.Models[def.Name] = def
}

func (m *Memory) PutAgent(def defs.AgentDefinition) {
	m.Agents[def.Name] = def
}

func (m *Memory) PutWorkflow(def defs.WorkflowDefinition) {
	m.Workflows[def.Name] = def
}

func (m *Memory) GetModel(name string) (defs.ModelDefinition, bool) {
	def, ok := m.Models[name]
	return def, ok
}

func (m *Memory) GetAgent(name string) (defs.AgentDefinition, bool) {
	def, ok := m.Agents[name]
	return def, ok
}

func (m *Memory) GetWorkflow(name string) (defs.WorkflowDefinition, bool) {
	def, ok := m.Workflows[name]
	return def, ok
}

func (m *Memory) Reload(raw []byte) error {
	return LoadIntoMemory(m, raw)
}
