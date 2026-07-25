package provider

import (
	"github.com/comalice/maelstrom/internal/prompt"
	"github.com/comalice/maelstrom/internal/runtime"
)

type FakeProvider struct {
	BuildErr     error
	SendErr      error
	BuiltRequest Request
	SentRequest  Request
	Response     Response
}

func (f *FakeProvider) BuildRequest(agent runtime.Agent, payload prompt.Payload, tools []ToolDefinition) (Request, error) {
	request, err := BuildRequest(agent, payload, tools)
	if err != nil {
		return Request{}, err
	}
	f.BuiltRequest = request
	if f.BuildErr != nil {
		return Request{}, f.BuildErr
	}
	return request, nil
}

func (f *FakeProvider) Send(request Request) (Response, error) {
	f.SentRequest = request
	if f.SendErr != nil {
		return Response{}, f.SendErr
	}
	return f.Response, nil
}
