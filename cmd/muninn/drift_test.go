package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

// --- refKeys ---

func TestRefKeys_FiltersAndSorts(t *testing.T) {
	resolved := map[string]any{
		"log_level":       "debug",
		"db_password_ref": "vault://secret/data/arasaka/db-password",
		"api_key_ref":     "vault://secret/data/arasaka/api-key",
		"db_password_key": "value", // not a *_ref key, must be excluded
	}

	got := refKeys(resolved)
	want := []string{"api_key_ref", "db_password_ref"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			break
		}
	}
}

func TestRefKeys_NoRefKeys_ReturnsNil(t *testing.T) {
	got := refKeys(map[string]any{"log_level": "debug"})
	if got != nil {
		t.Errorf("expected nil for no *_ref keys, got %v", got)
	}
}

func TestRefKeys_EmptyInput_ReturnsNil(t *testing.T) {
	if got := refKeys(nil); got != nil {
		t.Errorf("expected nil for nil input, got %v", got)
	}
}

// --- newlyAppearedRefs ---

func TestNewlyAppearedRefs_DetectsAddition(t *testing.T) {
	got := newlyAppearedRefs([]string{"db_password_ref"}, []string{"db_password_ref", "api_key_ref"})
	want := []string{"api_key_ref"}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNewlyAppearedRefs_NoChange_ReturnsEmpty(t *testing.T) {
	got := newlyAppearedRefs([]string{"db_password_ref"}, []string{"db_password_ref"})
	if len(got) != 0 {
		t.Errorf("expected no additions, got %v", got)
	}
}

func TestNewlyAppearedRefs_RemovalOnly_NotReportedAsAddition(t *testing.T) {
	// A key disappearing is not "newly appeared" - this function only ever
	// reports additions, since only additions require a Pod restart to
	// pick up (removing a ref doesn't affect an already-mounted secret).
	got := newlyAppearedRefs([]string{"db_password_ref", "api_key_ref"}, []string{"db_password_ref"})
	if len(got) != 0 {
		t.Errorf("expected no additions for a pure removal, got %v", got)
	}
}

func TestNewlyAppearedRefs_EmptyPrevious_EverythingIsNew(t *testing.T) {
	got := newlyAppearedRefs(nil, []string{"db_password_ref"})
	if len(got) != 1 || got[0] != "db_password_ref" {
		t.Errorf("got %v, want [db_password_ref]", got)
	}
}

// --- driftReporter ---

func TestDriftReporter_Report_OutsideCluster_LogsOnlyNoPanic(t *testing.T) {
	// This test process is not running in a Pod, so rest.InClusterConfig()
	// is expected to fail - Report must degrade to log-only, not panic or
	// error, matching the whole point of making Event emission best-effort.
	stderr := captureStderr(t)

	r := &driftReporter{}
	r.Report(context.Background(), "arasaka", []string{"api_key_ref"})

	out := stderr()
	if !strings.Contains(out, "drift: new secret reference(s) api_key_ref appeared for namespace arasaka") {
		t.Errorf("expected a drift log line, got: %s", out)
	}
	if r.clientset != nil {
		t.Error("expected clientset to remain nil outside a cluster")
	}
}

func TestDriftReporter_Report_NoPodIdentity_SkipsEventSilently(t *testing.T) {
	// Even if a clientset were somehow available, missing POD_NAME/
	// POD_NAMESPACE means there's no Pod to attribute the Event to -
	// Report must still log and return cleanly, not error.
	stderr := captureStderr(t)

	r := &driftReporter{}
	r.once.Do(func() {}) // pretend init already ran, clientset left nil
	r.Report(context.Background(), "arasaka", []string{"db_password_ref"})

	out := stderr()
	if !strings.Contains(out, "drift:") {
		t.Errorf("expected a drift log line regardless of Event capability, got: %s", out)
	}
}

func captureStderr(t *testing.T) func() string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = orig })

	return func() string {
		_ = w.Close()
		data, _ := io.ReadAll(r)
		os.Stderr = orig
		return string(data)
	}
}
