package kube

import (
	"testing"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/garoze/muninn/internal/app"
	"github.com/garoze/muninn/internal/config"
)

// These tests build the real Fx dependency graph (unlike the rest of this
// package, which constructs Watcher/DiscoveryService directly) - only this
// path exercises value-group wiring mistakes like a missing fx.As or
// fx.ParamTags.

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

// fxGraphWithFilter builds the fx.Provide chain shared by the tests below.
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
// Describe reports only enabled sources, not just registered ones.
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
