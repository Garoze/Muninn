package webhook

import (
	"sort"
	"strings"

	"go.uber.org/zap"
)

// SecretRef is a single secret reference extracted from a namespace's
// resolved configuration - the input SecretProviderClass generation
// (secretproviderclass.go) builds from.
type SecretRef struct {
	Key        string // resolved config_key, "_ref" suffix included
	ObjectName string // Key with "_ref" stripped - the SPC object's objectName
	Provider   string // URI scheme (e.g. "vault")
	Path       string // scheme-less remainder of the URI
	SecretKey  string // sibling *_key value; empty means "not given", not an error
}

const (
	// RefKeySuffix marks a resolved config key as a secret reference.
	// Exported because the sidecar's drift detection matches on the same
	// convention and must not carry a second copy of it.
	RefKeySuffix    = "_ref"
	secretKeySuffix = "_key"
)

// providers lists the URI schemes a reference may name. The scheme is the
// documented extension point for supporting a second secret store; only vault
// is implemented (see docs/adr/0012-csi-secret-delivery.md).
var providers = map[string]bool{
	"vault": true,
}

// ExtractSecretRefs pulls *_ref-suffixed string values out of resolved and
// parses each into a SecretRef.
//
// An unrecognized scheme or malformed value skips that reference with a warning
// rather than failing extraction, matching how the handler treats data it
// parsed but cannot act on.
func ExtractSecretRefs(resolved map[string]any, log *zap.Logger) []SecretRef {
	keys := make([]string, 0, len(resolved))
	for k := range resolved {
		keys = append(keys, k)
	}

	sort.Strings(keys) // deterministic order for idempotent SPC generation

	refs := make([]SecretRef, 0)
	for _, k := range keys {
		if !strings.HasSuffix(k, RefKeySuffix) {
			continue
		}

		v, ok := resolved[k].(string)
		if !ok {
			log.Warn("skipping non-string *_ref value",
				zap.String("key", k),
			)
			continue
		}

		scheme, path, ok := strings.Cut(v, "://")
		if !ok {
			log.Warn("skipping malformed secret reference (missing scheme)",
				zap.String("key", k),
			)
			continue
		}

		if !providers[scheme] {
			log.Warn("skipping secret reference with unrecognized scheme",
				zap.String("key", k),
				zap.String("scheme", scheme),
			)
			continue
		}

		// ObjectName becomes the mounted filename, so a key that is nothing but
		// the suffix leaves the driver nothing to write under, and "." or ".."
		// name a directory rather than a file. ConfigMap key validation already
		// excludes a separator, so those three are the whole set.
		objectName := strings.TrimSuffix(k, RefKeySuffix)
		if objectName == "" || objectName == "." || objectName == ".." {
			log.Warn("skipping secret reference whose name would not be a file",
				zap.String("key", k),
			)
			continue
		}

		refs = append(refs, SecretRef{
			Key:        k,
			ObjectName: objectName,
			Provider:   scheme,
			Path:       path,
			SecretKey:  secretKeyValue(resolved, objectName),
		})
	}

	return refs
}

// secretKeyValue looks up the optional sibling *_key entry for objectName.
// Absent or non-string means no field name was given, not an error.
func secretKeyValue(resolved map[string]any, objectName string) string {
	v, ok := resolved[objectName+secretKeySuffix]
	if !ok {
		return ""
	}

	s, _ := v.(string)
	return s
}
