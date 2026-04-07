package tools

import (
	"context"
	"testing"

	"github.com/Hochfrequenz/mcp-server-abap/adt"
	"github.com/mark3labs/mcp-go/mcp"
)

type captureToolAdder struct {
	tools []mcp.Tool
}

func (c *captureToolAdder) AddTool(tool mcp.Tool, _ func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)) {
	c.tools = append(c.tools, tool)
}

type activateToolClientStub struct{}

func (activateToolClientStub) CreateObject(context.Context, string, string, string, string, string) error {
	return nil
}

func (activateToolClientStub) CreateFunctionModule(context.Context, string, string, string, string, string) error {
	return nil
}

func (activateToolClientStub) CreatePackage(context.Context, string, string, string, string, string, string) error {
	return nil
}

func (activateToolClientStub) DeleteObject(context.Context, string, string, string) error {
	return nil
}

func (activateToolClientStub) ActivateObjects(context.Context, []string) (*adt.ActivationResult, error) {
	return &adt.ActivationResult{}, nil
}

func (activateToolClientStub) GetInactiveObjects(context.Context) ([]adt.ObjectInfo, error) {
	return nil, nil
}

func TestRegisterActivateTools_ObjectURIsArrayHasStringItems(t *testing.T) {
	adder := &captureToolAdder{}
	registerActivateTools(adder, activateToolClientStub{})

	var activateObjectsTool *mcp.Tool
	for i := range adder.tools {
		if adder.tools[i].Name == "activate_objects" {
			activateObjectsTool = &adder.tools[i]
			break
		}
	}
	if activateObjectsTool == nil {
		t.Fatal("activate_objects tool not found")
	}

	prop, ok := activateObjectsTool.InputSchema.Properties["object_uris"].(map[string]any)
	if !ok {
		t.Fatal("object_uris property missing or wrong type")
	}
	items, ok := prop["items"].(map[string]any)
	if !ok {
		t.Fatal("object_uris.items missing or wrong type")
	}
	if got := items["type"]; got != "string" {
		t.Fatalf("object_uris.items.type: got %v, want string", got)
	}
}
