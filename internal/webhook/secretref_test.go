package webhook

import (
	"reflect"
	"testing"

	"go.uber.org/zap"
)

func TestExtractSecretRefs_ValidRef_Extracted(t *testing.T) {
	resolved := map[string]any{
		"log_level":       "debug",
		"db_password_ref": "vault://secret/data/prod/db-password",
	}

	got := ExtractSecretRefs(resolved, zap.NewNop())

	want := []SecretRef{
		{
			Key:        "db_password_ref",
			ObjectName: "db_password",
			Provider:   "vault",
			Path:       "secret/data/prod/db-password",
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestExtractSecretRefs_MultipleRefs_SortedByKey(t *testing.T) {
	resolved := map[string]any{
		"stripe_key_ref":  "vault://secret/data/prod/stripe",
		"api_key_ref":     "vault://secret/data/prod/api",
		"db_password_ref": "vault://secret/data/prod/db",
	}

	got := ExtractSecretRefs(resolved, zap.NewNop())

	if len(got) != 3 {
		t.Fatalf("got %d refs, want 3: %+v", len(got), got)
	}

	wantOrder := []string{"api_key_ref", "db_password_ref", "stripe_key_ref"}
	for i, key := range wantOrder {
		if got[i].Key != key {
			t.Errorf("index %d: got key %q, want %q (sorted order not deterministic)", i, got[i].Key, key)
		}
	}
}

func TestExtractSecretRefs_NonRefKeys_Ignored(t *testing.T) {
	resolved := map[string]any{
		"log_level": "debug",
		"api_url":   "https://api.example.com",
		"db_port":   "5432",
	}

	got := ExtractSecretRefs(resolved, zap.NewNop())

	if len(got) != 0 {
		t.Errorf("expected no refs extracted from non-_ref keys, got %+v", got)
	}
}

func TestExtractSecretRefs_EmptyResolved_ReturnsEmpty(t *testing.T) {
	got := ExtractSecretRefs(map[string]any{}, zap.NewNop())
	if len(got) != 0 {
		t.Errorf("expected empty result for empty input, got %+v", got)
	}
}

func TestExtractSecretRefs_NilResolved_ReturnsEmpty(t *testing.T) {
	got := ExtractSecretRefs(nil, zap.NewNop())
	if len(got) != 0 {
		t.Errorf("expected empty result for nil input, got %+v", got)
	}
}

func TestExtractSecretRefs_SkipsNegativeCases(t *testing.T) {
	tests := []struct {
		name     string
		resolved map[string]any
	}{
		{
			name:     "unrecognized scheme",
			resolved: map[string]any{"db_password_ref": "ssm://prod/db-password"},
		},
		{
			name:     "missing scheme (no ://)",
			resolved: map[string]any{"db_password_ref": "just-a-plain-string"},
		},
		{
			name:     "empty string value",
			resolved: map[string]any{"db_password_ref": ""},
		},
		{
			name:     "non-string value (int)",
			resolved: map[string]any{"db_password_ref": 42},
		},
		{
			name:     "non-string value (bool)",
			resolved: map[string]any{"db_password_ref": true},
		},
		{
			name:     "non-string value (nested map)",
			resolved: map[string]any{"db_password_ref": map[string]any{"nested": "value"}},
		},
		{
			name:     "empty scheme (leading ://)",
			resolved: map[string]any{"db_password_ref": "://secret/data/prod/db-password"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSecretRefs(tt.resolved, zap.NewNop())
			if len(got) != 0 {
				t.Errorf("expected ref to be skipped, got %+v", got)
			}
		})
	}
}

func TestExtractSecretRefs_MixedValidAndInvalid_OnlyValidExtracted(t *testing.T) {
	resolved := map[string]any{
		"log_level":       "debug",
		"db_password_ref": "vault://secret/data/prod/db-password",
		"api_key_ref":     "ssm://prod/api-key", // unrecognized scheme, skipped
		"broken_ref":      "not-a-uri",          // missing scheme, skipped
	}

	got := ExtractSecretRefs(resolved, zap.NewNop())

	if len(got) != 1 {
		t.Fatalf("got %d refs, want 1: %+v", len(got), got)
	}
	if got[0].Key != "db_password_ref" {
		t.Errorf("got key %q, want db_password_ref", got[0].Key)
	}
}

func TestExtractSecretRefs_PathPreservesEmbeddedSchemeDelimiters(t *testing.T) {
	// strings.Cut splits on the first "://" only - anything after that in
	// the value is opaque path data the provider owns, not something this
	// extractor should try to further parse.
	resolved := map[string]any{
		"weird_ref": "vault://secret/data/prod/db?redirect=https://example.com",
	}

	got := ExtractSecretRefs(resolved, zap.NewNop())

	if len(got) != 1 {
		t.Fatalf("got %d refs, want 1: %+v", len(got), got)
	}
	want := "secret/data/prod/db?redirect=https://example.com"
	if got[0].Path != want {
		t.Errorf("got Path %q, want %q", got[0].Path, want)
	}
}

// --- the optional sibling *_key field ---
//
// A secret backend stores a map at a path, not a scalar, so the field to
// extract is named separately rather than assumed. Absent means "no field
// given", which the driver reads as "mount the whole secret as JSON" - a safe
// default rather than an error.

func TestExtractSecretRefs_SiblingKeyIsCarried(t *testing.T) {
	resolved := map[string]any{
		"db_password_ref": "vault://secret/data/prod/db-password",
		"db_password_key": "value",
	}

	got := ExtractSecretRefs(resolved, zap.NewNop())
	if len(got) != 1 {
		t.Fatalf("got %d refs, want 1: %+v", len(got), got)
	}
	if got[0].SecretKey != "value" {
		t.Errorf("SecretKey: got %q, want %q", got[0].SecretKey, "value")
	}
	// The *_key entry is a sibling, not a reference of its own.
	if got[0].Key != "db_password_ref" {
		t.Errorf("Key: got %q, want db_password_ref", got[0].Key)
	}
}

func TestExtractSecretRefs_NoSiblingKeyMeansUnset(t *testing.T) {
	resolved := map[string]any{"db_password_ref": "vault://secret/data/prod/db-password"}

	got := ExtractSecretRefs(resolved, zap.NewNop())
	if len(got) != 1 {
		t.Fatalf("got %d refs, want 1: %+v", len(got), got)
	}
	if got[0].SecretKey != "" {
		t.Errorf("SecretKey: got %q, want empty (absent is not an error)", got[0].SecretKey)
	}
}

func TestExtractSecretRefs_NonStringSiblingKeyIsIgnored(t *testing.T) {
	resolved := map[string]any{
		"db_password_ref": "vault://secret/data/prod/db-password",
		"db_password_key": 42,
	}

	got := ExtractSecretRefs(resolved, zap.NewNop())
	if len(got) != 1 {
		t.Fatalf("got %d refs, want 1: %+v", len(got), got)
	}
	if got[0].SecretKey != "" {
		t.Errorf("SecretKey: got %q, want empty for a non-string value", got[0].SecretKey)
	}
}

// Each ref takes only its own sibling, matched on the shared prefix.
func TestExtractSecretRefs_SiblingKeysMatchTheirOwnRef(t *testing.T) {
	resolved := map[string]any{
		"db_password_ref": "vault://secret/data/prod/db",
		"db_password_key": "password",
		"api_token_ref":   "vault://secret/data/prod/api",
		"api_token_key":   "token",
	}

	got := ExtractSecretRefs(resolved, zap.NewNop())
	if len(got) != 2 {
		t.Fatalf("got %d refs, want 2: %+v", len(got), got)
	}

	byObject := map[string]string{}
	for _, r := range got {
		byObject[r.ObjectName] = r.SecretKey
	}
	if byObject["db_password"] != "password" || byObject["api_token"] != "token" {
		t.Errorf("sibling keys crossed between refs: %+v", byObject)
	}
}

// A key that is nothing but the suffix leaves no object name for the driver to
// write a file under, so it is not a usable reference.
func TestExtractSecretRefs_BareSuffixKeyIsSkipped(t *testing.T) {
	resolved := map[string]any{"_ref": "vault://secret/data/prod/db"}

	if got := ExtractSecretRefs(resolved, zap.NewNop()); len(got) != 0 {
		t.Errorf("expected a bare %q key to be skipped, got %+v", "_ref", got)
	}
}
