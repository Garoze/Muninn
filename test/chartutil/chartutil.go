// Package chartutil gives the test tiers one way to locate the Helm chart
// and one place that makes its dependencies present on disk.
package chartutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"sigs.k8s.io/yaml"
)

var (
	once     sync.Once
	buildErr error
)

// Path resolves the chart directory from this file's own location, so it
// holds whatever working directory a test binary runs from.
func Path(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "charts", "muninn")
}

// EnsureDependencies vendors the chart's subchart archives into its own
// charts/ directory, skipping the calling test if helm is absent. Helm
// refuses to render or install a chart whose declared dependencies are
// missing from disk even when every one of them is condition-disabled, as
// they are here, and those archives are gitignored.
func EnsureDependencies(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not found in PATH, required to render or install the chart")
	}
	chartDir := Path(t)
	once.Do(func() { buildErr = buildDependencies(chartDir) })
	if buildErr != nil {
		t.Fatalf("vendor chart dependencies: %v", buildErr)
	}
}

// buildDependencies registers the chart's declared repositories into a
// throwaway Helm configuration and builds against it. `helm dependency
// build` resolves each dependency through helm's registered repository
// list, not through the URL in Chart.yaml, so the repositories have to be
// added first - but doing that against the caller's own configuration would
// mean a test editing a developer's repo list.
func buildDependencies(chartDir string) error {
	repos, err := declaredRepositories(chartDir)
	if err != nil {
		return err
	}

	home, err := os.MkdirTemp("", "muninn-helm-")
	if err != nil {
		return fmt.Errorf("temporary helm home: %w", err)
	}
	defer func() { _ = os.RemoveAll(home) }()

	env := append(os.Environ(),
		"HELM_REPOSITORY_CONFIG="+filepath.Join(home, "repositories.yaml"),
		"HELM_REPOSITORY_CACHE="+filepath.Join(home, "cache"),
	)

	for name, url := range repos {
		if err := run(env, "helm", "repo", "add", name, url); err != nil {
			return err
		}
	}
	return run(env, "helm", "dependency", "build", chartDir)
}

// declaredRepositories reads the dependency repositories out of Chart.yaml
// rather than restating them here, so this cannot drift from what the chart
// actually declares. Non-HTTP references (a vendored file://, an OCI
// registry) need no repository entry and are left alone.
func declaredRepositories(chartDir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(chartDir, "Chart.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read Chart.yaml: %w", err)
	}

	var meta struct {
		Dependencies []struct {
			Name       string `json:"name"`
			Repository string `json:"repository"`
		} `json:"dependencies"`
	}
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("decode Chart.yaml: %w", err)
	}

	repos := map[string]string{}
	for _, dep := range meta.Dependencies {
		if strings.HasPrefix(dep.Repository, "http") {
			repos[dep.Name] = dep.Repository
		}
	}
	return repos, nil
}

func run(env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, out)
	}
	return nil
}
