package tools

import (
	"context"
	"testing"
)

func TestRegistryClassifiesInvalidToolInputAsRecoverableRejection(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(Spec{
		Name:        "example.read",
		Description: "Read one example.",
		InputSchema: Schema{
			Type:       "object",
			Required:   []string{"query"},
			Properties: map[string]Schema{"query": {Type: "string"}},
		},
		OutputSchema: Schema{Type: "object", AdditionalProperties: true},
	}, func(context.Context, Call) (Result, error) {
		return Result{Output: map[string]any{}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = registry.Execute(context.Background(), Call{Name: "example.read", Input: map[string]any{"query": 42}}, ExecuteOptions{
		Allowed:       []string{"example.read"},
		RequireListed: true,
	})
	if !IsCallRejected(err) {
		t.Fatalf("invalid input error=%v, want recoverable call rejection", err)
	}
}

func TestRegistryPermissionFailureIsNotRecoverable(t *testing.T) {
	registry := NewRegistry()
	err := registry.Register(Spec{
		Name:         "example.read",
		Description:  "Read one example.",
		InputSchema:  Schema{Type: "object", AdditionalProperties: true},
		OutputSchema: Schema{Type: "object", AdditionalProperties: true},
	}, func(context.Context, Call) (Result, error) {
		return Result{Output: map[string]any{}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = registry.Execute(context.Background(), Call{Name: "example.read", Input: map[string]any{}}, ExecuteOptions{
		Allowed:       []string{"other.read"},
		RequireListed: true,
	})
	if err == nil || IsCallRejected(err) {
		t.Fatalf("permission error=%v, want hard policy failure", err)
	}
}
