package kube

import (
	"testing"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
)

// These tests exist because internal/kube/module.go shipped with two real Fx
// wiring bugs (fixed 2026-08-04) that nothing else in this package caught:
// NewConfigMapSource was provided into the "config_sources" value group under
// its concrete type with no fx.As(new(ConfigSource)), and
// provideConfigSourceDescriptors had no fx.ParamTags at all, so its
// []ConfigSource parameter resolved against a nonexistent ungrouped provider
// instead of the group. Every other test in this package constructs
// Watcher/DiscoveryService directly, bypassing Fx entirely - these are the
// only tests that build the real dependency graph, the same way muninn serve
// does.

func testConfig() *config.Config {
	return &config.Config{ConfigMapLabelSelector: "muninn.io/config=runtime"}
}

func TestConfigSourcesGroup_ResolvesAsInterfaceSlice(t *testing.T) {
	var sources []ConfigSource

	fxApp := fxtest.New(t,
		fx.Provide(testConfig),
		fx.Provide(
			fx.Annotate(NewConfigMapSource, fx.As(new(ConfigSource)), fx.ResultTags(`group:"config_sources"`)),
		),
		fx.Populate(fx.Annotate(&sources, fx.ParamTags(`group:"config_sources"`))),
	)
	defer fxApp.RequireStart().RequireStop()

	if len(sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(sources))
	}
	if sources[0].Kind() != "ConfigMap" {
		t.Errorf("Kind: got %q, want ConfigMap", sources[0].Kind())
	}
}

// TestConfigSourcesGroup_MultipleSourcesCompose is the Fx-graph-level version
// of watcher_test.go's TestOnSourceUpsert_DifferentKindsDoNotCollide: proof
// that a second ConfigSource really does slot in as "one more fx.Provide,"
// the claim internal/kube/module.go's own comment makes.
func TestConfigSourcesGroup_MultipleSourcesCompose(t *testing.T) {
	var sources []ConfigSource

	fxApp := fxtest.New(t,
		fx.Provide(testConfig),
		fx.Provide(
			fx.Annotate(NewConfigMapSource, fx.As(new(ConfigSource)), fx.ResultTags(`group:"config_sources"`)),
		),
		fx.Provide(
			fx.Annotate(newFakeSourceForFx, fx.As(new(ConfigSource)), fx.ResultTags(`group:"config_sources"`)),
		),
		fx.Populate(fx.Annotate(&sources, fx.ParamTags(`group:"config_sources"`))),
	)
	defer fxApp.RequireStart().RequireStop()

	if len(sources) != 2 {
		t.Fatalf("got %d sources, want 2", len(sources))
	}
}

// newFakeSourceForFx adapts watcher_test.go's fakeSource to a zero-arg Fx
// constructor.
func newFakeSourceForFx() ConfigSource {
	return &fakeSource{kind: "Fake"}
}

// fxGraphWithFilter builds the fx.Provide chain shared by the tests below:
// the raw "config_sources" group, through filterEnabledSources, matching
// how internal/kube/module.go actually wires both NewWatcher and
// provideConfigSourceDescriptors today.
func fxGraphWithFilter(cfg *config.Config, extra ...fx.Option) []fx.Option {
	return append([]fx.Option{
		fx.Provide(func() *config.Config { return cfg }),
		fx.Provide(
			fx.Annotate(NewConfigMapSource, fx.As(new(ConfigSource)), fx.ResultTags(`group:"config_sources"`)),
		),
		fx.Provide(
			fx.Annotate(filterEnabledSources, fx.ParamTags("", `group:"config_sources"`)),
		),
	}, extra...)
}

func TestProvideConfigSourceDescriptors_PullsFromFilteredSources(t *testing.T) {
	var descriptors []app.ConfigSourceDescriptor

	fxApp := fxtest.New(t, append(
		fxGraphWithFilter(testConfig(), fx.Provide(provideConfigSourceDescriptors)),
		fx.Populate(&descriptors),
	)...)
	defer fxApp.RequireStart().RequireStop()

	if len(descriptors) != 1 {
		t.Fatalf("got %d descriptors, want 1", len(descriptors))
	}
	want := app.ConfigSourceDescriptor{Kind: "ConfigMap", LabelSelector: "muninn.io/config=runtime", Scope: "namespace"}
	if descriptors[0] != want {
		t.Errorf("got %+v, want %+v", descriptors[0], want)
	}
}

// TestProvideConfigSourceDescriptors_ExcludedSourceIsNotDescribed proves
// Describe only reports enabled sources, not just registered ones - the
// filter sits ahead of provideConfigSourceDescriptors too, not only
// NewWatcher.
func TestProvideConfigSourceDescriptors_ExcludedSourceIsNotDescribed(t *testing.T) {
	var descriptors []app.ConfigSourceDescriptor

	cfg := testConfig()
	cfg.EnabledConfigSources = []string{"SomethingElse"}

	fxApp := fxtest.New(t, append(
		fxGraphWithFilter(cfg, fx.Provide(provideConfigSourceDescriptors)),
		fx.Populate(&descriptors),
	)...)
	defer fxApp.RequireStart().RequireStop()

	if len(descriptors) != 0 {
		t.Fatalf("got %d descriptors, want 0 (ConfigMap source excluded)", len(descriptors))
	}
}

func TestFilterEnabledSources_EmptyMeansAllEnabled(t *testing.T) {
	sources := []ConfigSource{&fakeSource{kind: "A"}, &fakeSource{kind: "B"}}

	got := filterEnabledSources(&config.Config{}, sources)
	if len(got) != 2 {
		t.Fatalf("got %d sources, want 2 (no filter configured)", len(got))
	}
}

func TestFilterEnabledSources_FiltersByKind(t *testing.T) {
	sources := []ConfigSource{&fakeSource{kind: "A"}, &fakeSource{kind: "B"}}

	got := filterEnabledSources(&config.Config{EnabledConfigSources: []string{"B"}}, sources)
	if len(got) != 1 || got[0].Kind() != "B" {
		t.Fatalf("got %+v, want only kind B", got)
	}
}
