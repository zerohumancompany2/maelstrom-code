package binding

import "fmt"

type Record struct {
	ID         string
	AgentID    string
	WorkflowID string
	Action     string
	Reason     string
}

type Log struct {
	Records []Record
	count   int
}

func NewLog() *Log {
	return &Log{}
}

func (l *Log) NextID() string {
	l.count++
	return fmt.Sprintf("bind-%03d", l.count)
}

func (l *Log) Append(record Record) {
	l.Records = append(l.Records, record)
}
