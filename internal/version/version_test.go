package version_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDefault guards the unstamped fallback plain `go build`/`go run`
// relies on: if this ever silently becomes "" or something else, every
// local dev build starts reporting a version without anyone changing it
// on purpose.
func TestDefault(t *testing.T) {
	out := buildAndRun(t, "")
	if out != "dev" {
		t.Errorf("unstamped build: got version %q, want %q", out, "dev")
	}
}

// TestLdflagsStamping is the actual proof for the risk -ldflags -X carries:
// -X targets a fully-qualified symbol path, so renaming the package or the
// variable makes the stamp silently become an empty string - the build
// still succeeds and nothing fails. A default-value-only test would not
// catch that regression; this rebuilds the binary the same way `make
// build` does and asserts the stamped value actually reaches the binary.
func TestLdflagsStamping(t *testing.T) {
	const want = "v0.0.0-test"
	out := buildAndRun(t, want)
	if out != want {
		t.Errorf("ldflags-stamped build: got version %q, want %q", out, want)
	}
}

// buildAndRun compiles cmd/muninnctl with the given version (via -ldflags
// -X, matching the Makefile's build target) into a temp dir, runs `version`,
// and returns the trimmed stdout. An empty version skips -ldflags entirely,
// exercising the plain `go build` path.
func buildAndRun(t *testing.T, ldflagsVersion string) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "muninnctl")
	args := []string{"build", "-o", bin}
	if ldflagsVersion != "" {
		args = append(args, "-ldflags", "-X github.com/garoze/muninn/internal/version.Version="+ldflagsVersion)
	}
	args = append(args, "../../cmd/muninnctl")

	build := exec.Command("go", args...)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	run := exec.Command(bin, "version")
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("%s version: %v\n%s", bin, err, out)
	}

	return strings.TrimSpace(string(out))
}
