package main

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	discoveryv1 "github.com/garoze/muninn/gen/discovery/v1"
)

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
