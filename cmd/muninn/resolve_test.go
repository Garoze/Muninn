package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
	"sigs.k8s.io/yaml"

	discoveryv1 "github.com/garoze/muninn/gen/discovery/v1"
)

// fakeDiscoveryClient implements discoveryv1.DiscoveryServiceClient without a
// live gRPC server. responses/errs are consumed in order by Resolve calls;
// once exhausted, the last entry repeats (so a --watch loop that outlives the
// script keeps getting a deterministic answer instead of a panic).
type fakeDiscoveryClient struct {
	responses []*discoveryv1.ResolveResponse
	errs      []error
	calls     int
}

func (f *fakeDiscoveryClient) Resolve(context.Context, *discoveryv1.ResolveRequest, ...grpc.CallOption) (*discoveryv1.ResolveResponse, error) {
	i := f.calls
	if i >= len(f.responses) {
		i = len(f.responses) - 1
	}
	f.calls++
	return f.responses[i], f.errs[i]
}

func (f *fakeDiscoveryClient) Query(context.Context, *discoveryv1.QueryRequest, ...grpc.CallOption) (*discoveryv1.QueryResponse, error) {
	panic("not used by resolve.go")
}

func (f *fakeDiscoveryClient) Describe(context.Context, *discoveryv1.DescribeRequest, ...grpc.CallOption) (*discoveryv1.DescribeResponse, error) {
	panic("not used by resolve.go")
}

func mustKeyValue(t *testing.T, key string, value any) *discoveryv1.KeyValue {
	t.Helper()
	v, err := structpb.NewValue(value)
	if err != nil {
		t.Fatalf("structpb.NewValue(%v): %v", value, err)
	}
	return &discoveryv1.KeyValue{Key: key, Value: v}
}

// --- marshalConfigFile ---

func TestMarshalConfigFile(t *testing.T) {
	resp := &discoveryv1.ResolveResponse{
		Values: []*discoveryv1.KeyValue{
			mustKeyValue(t, "LOG_LEVEL", "info"),
			mustKeyValue(t, "FEATURE_FLAG", "true"),
		},
	}

	data, err := marshalConfigFile(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got map[string]any
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if got["LOG_LEVEL"] != "info" || got["FEATURE_FLAG"] != "true" {
		t.Errorf("got %+v", got)
	}
}

func TestMarshalConfigFile_Empty(t *testing.T) {
	data, err := marshalConfigFile(&discoveryv1.ResolveResponse{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected some output for an empty response, got none")
	}
}

// --- writeFileAtomic ---

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := writeFileAtomic(path, []byte("a: 1\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "a: 1\n" {
		t.Errorf("got %q, want %q", got, "a: 1\n")
	}
}

func TestWriteFileAtomic_Overwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := writeFileAtomic(path, []byte("a: 1\n")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := writeFileAtomic(path, []byte("a: 2\n")); err != nil {
		t.Fatalf("second write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "a: 2\n" {
		t.Errorf("got %q, want %q", got, "a: 2\n")
	}

	// No leftover .tmp file - the temp file was renamed, not left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 file in dir after two writes, got %d: %v", len(entries), entries)
	}
}

func TestWriteFileAtomic_NonexistentDirErrors(t *testing.T) {
	err := writeFileAtomic(filepath.Join(t.TempDir(), "does-not-exist", "config.yaml"), []byte("a: 1\n"))
	if err == nil {
		t.Fatal("expected an error for a nonexistent directory, got nil")
	}
}

// --- pollFailure ---

func TestPollFailure_FileExists_LogsAndReturnsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("a: 1\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := pollFailure(path, "boom", errors.New("underlying")); err != nil {
		t.Errorf("got %v, want nil (file exists - recoverable)", err)
	}
}

func TestPollFailure_FileMissing_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	err := pollFailure(path, "boom", errors.New("underlying"))
	if err == nil {
		t.Fatal("expected an error (no file to fall back on), got nil")
	}
	if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "underlying") {
		t.Errorf("got %q, want it to mention both the message and the underlying error", err.Error())
	}
}

// --- resolveOnce ---

func TestResolveOnce_WritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	client := &fakeDiscoveryClient{
		responses: []*discoveryv1.ResolveResponse{
			{Values: []*discoveryv1.KeyValue{mustKeyValue(t, "LOG_LEVEL", "info")}, Revision: "1"},
		},
		errs: []error{nil},
	}

	if err := resolveOnce(context.Background(), client, "ns1", path); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got map[string]any
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if got["LOG_LEVEL"] != "info" {
		t.Errorf("got %+v", got)
	}
}

func TestResolveOnce_PropagatesResolveError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	client := &fakeDiscoveryClient{
		responses: []*discoveryv1.ResolveResponse{nil},
		errs:      []error{errors.New("unavailable")},
	}

	if err := resolveOnce(context.Background(), client, "ns1", path); err == nil {
		t.Fatal("expected an error, got nil")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("expected no file to be written on a failed resolve")
	}
}

// --- resolveWatch ---

func TestResolveWatch_FirstPollFailureIsFatalWhenNoFileExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	client := &fakeDiscoveryClient{
		responses: []*discoveryv1.ResolveResponse{nil},
		errs:      []error{errors.New("connection refused")},
	}

	err := resolveWatch(client, "ns1", path, time.Second)
	if err == nil {
		t.Fatal("expected an error when the first poll fails and no file exists yet")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("expected no file to have been created")
	}
}

func TestResolveWatch_PollFailureWithExistingFileLogsAndContinues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("LOG_LEVEL: info\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	client := &fakeDiscoveryClient{
		responses: []*discoveryv1.ResolveResponse{nil},
		errs:      []error{errors.New("connection refused")},
	}

	done := make(chan error, 1)
	go func() {
		done <- resolveWatch(client, "ns1", path, 20*time.Millisecond)
	}()

	time.Sleep(80 * time.Millisecond) // let several failed polls happen
	stopResolveWatch(t, done)

	// Last-good (pre-existing) content stays in place - every poll failed,
	// but with a file already present, resolveWatch keeps retrying instead
	// of exiting or clearing the file.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "LOG_LEVEL: info\n" {
		t.Errorf("got %q, want original content untouched", data)
	}
}

func TestResolveWatch_ConvergesToLatestValueAcrossTicks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	client := &fakeDiscoveryClient{
		responses: []*discoveryv1.ResolveResponse{
			{Values: []*discoveryv1.KeyValue{mustKeyValue(t, "LOG_LEVEL", "info")}, Revision: "1"},
			{Values: []*discoveryv1.KeyValue{mustKeyValue(t, "LOG_LEVEL", "info")}, Revision: "1"},  // unchanged - should not rewrite
			{Values: []*discoveryv1.KeyValue{mustKeyValue(t, "LOG_LEVEL", "debug")}, Revision: "2"}, // changed
		},
		errs: []error{nil, nil, nil},
	}

	done := make(chan error, 1)
	go func() {
		done <- resolveWatch(client, "ns1", path, 20*time.Millisecond)
	}()

	// First tick is immediate; several more follow on the 20ms interval -
	// enough time for all three scripted responses to be consumed (and then
	// some, repeating the last one, which the unit test tolerates).
	time.Sleep(150 * time.Millisecond)
	stopResolveWatch(t, done)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got map[string]any
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if got["LOG_LEVEL"] != "debug" {
		t.Errorf("got %+v, want the final resolved value to have been written", got)
	}
}

// stopResolveWatch signals SIGTERM to the current process - the same signal
// resolveWatch's own signal.NotifyContext listens for, and the same one
// Kubernetes sends a sidecar container on pod termination - then waits for
// the resolveWatch goroutine under test to return.
func stopResolveWatch(t *testing.T, done <-chan error) {
	t.Helper()

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("signaling self: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error from resolveWatch: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resolveWatch did not exit after SIGTERM")
	}
}

// --- cmdResolve ---

func TestCmdResolve_MissingRequiredFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no flags", nil},
		{"namespace only", []string{"--namespace", "ns1"}},
		{"out only", []string{"--out", "/tmp/x.yaml"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cmdResolve(tt.args)
			if err == nil {
				t.Fatal("expected an error for missing --namespace/--out, got nil")
			}
			if !strings.Contains(err.Error(), "--namespace and --out are required") {
				t.Errorf("unexpected error message: %v", err)
			}
		})
	}
}
