package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	discoveryv1 "github.com/garoze/muninn/gen/discovery/v1"
)

// errWriter always fails, to exercise the error-propagation path (Flush's
// return value) rather than just the happy-path formatting.
type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestFormatQueryResponse(t *testing.T) {
	value, err := structpb.NewValue("acme-corp")
	if err != nil {
		t.Fatalf("structpb.NewValue: %v", err)
	}

	resp := &discoveryv1.QueryResponse{
		Values: []*discoveryv1.KeyValue{
			{Key: "TENANT.id", Value: value, Source: "Tenant"},
		},
		MissingKeys: []string{"TENANT.displayName"},
	}

	var buf bytes.Buffer
	if err := formatQueryResponse(&buf, resp); err != nil {
		t.Fatalf("formatQueryResponse: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "TENANT.id") || !strings.Contains(out, "acme-corp") || !strings.Contains(out, "Tenant") {
		t.Errorf("expected value row in output, got:\n%s", out)
	}
	if !strings.Contains(out, "missing: TENANT.displayName") {
		t.Errorf("expected missing keys line in output, got:\n%s", out)
	}
}

func TestFormatQueryResponse_NoMissingKeys(t *testing.T) {
	resp := &discoveryv1.QueryResponse{}

	var buf bytes.Buffer
	if err := formatQueryResponse(&buf, resp); err != nil {
		t.Fatalf("formatQueryResponse: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "missing:") {
		t.Errorf("expected no missing keys line, got:\n%s", out)
	}
}

func TestFormatDescribeResponse(t *testing.T) {
	resp := &discoveryv1.DescribeResponse{
		SupportedKeys: []*discoveryv1.SupportedKey{
			{Key: "TENANT.id", TypeHint: "string", Description: "Tenant identifier"},
		},
	}

	var buf bytes.Buffer
	if err := formatDescribeResponse(&buf, resp); err != nil {
		t.Fatalf("formatDescribeResponse: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, "TENANT.id") || !strings.Contains(out, "string") || !strings.Contains(out, "Tenant identifier") {
		t.Errorf("expected supported key row in output, got:\n%s", out)
	}
}

func TestFormatQueryResponse_WriteError(t *testing.T) {
	resp := &discoveryv1.QueryResponse{
		Values: []*discoveryv1.KeyValue{{Key: "TENANT.id", Source: "Tenant"}},
	}

	if err := formatQueryResponse(errWriter{}, resp); err == nil {
		t.Fatal("expected an error from a failing writer, got nil")
	}
}

func TestFormatDescribeResponse_WriteError(t *testing.T) {
	resp := &discoveryv1.DescribeResponse{
		SupportedKeys: []*discoveryv1.SupportedKey{{Key: "TENANT.id"}},
	}

	if err := formatDescribeResponse(errWriter{}, resp); err == nil {
		t.Fatal("expected an error from a failing writer, got nil")
	}
}

func TestCmdQuery_MissingRequiredFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no flags", nil},
		{"tenant only", []string{"--tenant", "arasaka"}},
		{"keys only", []string{"--keys", "TENANT.id"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cmdQuery(tt.args)
			if err == nil {
				t.Fatal("expected an error for missing --tenant/--keys, got nil")
			}
			if !strings.Contains(err.Error(), "--tenant and --keys are required") {
				t.Errorf("unexpected error message: %v", err)
			}
		})
	}
}
