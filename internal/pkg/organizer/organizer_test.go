package organizer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fredbi/benchviz/internal/pkg/config"
	"github.com/fredbi/benchviz/internal/pkg/parser"
	"golang.org/x/tools/benchmark/parse"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestNew(t *testing.T) {
	cfg := mustLoadConfig(t, minimalConfig())
	o := New(cfg)
	require.NotNil(t, o)
	assert.Equal(t, cfg, o.cfg)
}

func TestParseBenchmarkName(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	tests := []struct {
		name         string
		benchName    string
		file         string
		env          string
		wantOk       bool
		wantFunction string
		wantVersion  string
		wantContext  string
	}{
		{
			name:         "full match from name",
			benchName:    "BenchmarkGreater/reflect/int-16",
			wantOk:       true,
			wantFunction: "greater",
			wantVersion:  "reflect",
			wantContext:  "int",
		},
		{
			name:         "generic version",
			benchName:    "BenchmarkGreater/generic/float64-16",
			wantOk:       true,
			wantFunction: "greater",
			wantVersion:  "generics",
			wantContext:  "float64",
		},
		{
			name:         "less function",
			benchName:    "BenchmarkLess/reflect/int-16",
			wantOk:       true,
			wantFunction: "less",
			wantVersion:  "reflect",
			wantContext:  "int",
		},
		{
			name:         "excluded by NotMatch",
			benchName:    "BenchmarkGreaterOrEqual/reflect/int-16",
			wantOk:       false,
			wantFunction: "",
		},
		{
			name:         "no function match",
			benchName:    "BenchmarkUnknown/reflect/int-16",
			wantOk:       false,
			wantFunction: "",
		},
		{
			name:         "negative function",
			benchName:    "BenchmarkNegative/reflect/int-16",
			wantOk:       true,
			wantFunction: "negative",
			wantVersion:  "reflect",
			wantContext:  "int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, ok := o.parseBenchmarkName(tt.benchName, tt.file, tt.env)
			require.Equal(t, tt.wantOk, ok, "parseBenchmarkName(%q) ok", tt.benchName)
			if !ok {
				return
			}
			assert.Equal(t, tt.wantFunction, parsed.Function)
			assert.Equal(t, tt.wantVersion, parsed.Version)
			assert.Equal(t, tt.wantContext, parsed.Context)
		})
	}
}

// TestParseBenchmarkNameContextFallback verifies that when the context
// is not found in the benchmark name, it falls back to file-based matching.
func TestParseBenchmarkNameContextFallbackBug(t *testing.T) {
	cfg := mustLoadConfig(t, configWithFileFallback())
	o := New(cfg)

	// The benchmark name contains the function but NOT the version or context.
	// Both should fall back to file-based matching.
	parsed, ok := o.parseBenchmarkName(
		"BenchmarkGreater-16",       // no version/context in name
		"bench_reflect_int_test.go", // file should match version=reflect, context=int
		"linux amd64",
	)
	require.True(t, ok, "expected parseBenchmarkName to succeed")
	assert.Equal(t, "reflect", parsed.Version, "version file fallback")
	assert.Equal(t, "int", parsed.Context, "context file fallback")
}

func TestParseBenchmarks(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}

	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)
	require.NotEmpty(t, benchSet.Set)

	// The config has 2 metrics (nsPerOp, allocsPerOp).
	// The generics set has 4 benchmarks (Greater reflect/int, Greater generic/int,
	// Greater reflect/float64, Greater generic/float64).
	// Each benchmark should produce 2 ParsedBenchmarks (one per metric).
	// Total: 4 * 2 = 8
	assert.Len(t, benchSet.Set, 8)

	// Verify we have the right metrics
	metrics := make(map[config.MetricName]int)
	for _, b := range benchSet.Set {
		metrics[b.Metric]++
	}
	assert.Equal(t, 4, metrics[config.MetricNsPerOp])
	assert.Equal(t, 4, metrics[config.MetricAllocsPerOp])
}

func TestParseBenchmarksEmpty(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	benchSet, err := o.parseBenchmarks(nil)
	require.NoError(t, err)
	assert.Empty(t, benchSet.Set)
}

func TestParseBenchmarksSkipsUnmatched(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{{
		Set: parse.Set{
			"BenchmarkUnknown-16": []*parse.Benchmark{
				{Name: "BenchmarkUnknown-16", N: 1000, NsPerOp: 100},
			},
		},
	}}

	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)
	assert.Empty(t, benchSet.Set)
}

func TestSeriesFor(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)

	category := cfg.Categories[0]

	series := benchSet.SeriesFor(config.MetricNsPerOp, "reflect", category)

	require.NotEmpty(t, series)

	// For version "reflect", the category includes function "greater"
	// and contexts "int" and "float64" → 1 series with 2 points.
	require.Len(t, series, 1)

	s := series[0]
	assert.Equal(t, "reflect", s.Title)
	assert.Len(t, s.Points, 2)
	for _, p := range s.Points {
		assert.Positive(t, p.Value, "expected positive value for %q", p.Name)
	}
}

func TestSeriesForNoMatch(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)

	category := cfg.Categories[0]

	// Query a version that doesn't exist in the data
	series := benchSet.SeriesFor(config.MetricNsPerOp, "nonexistent", category)
	assert.NotEmpty(t, series)
}

// TestPopulateCategories verifies that populateCategories produces
// exactly the right number of categories.
func TestPopulateCategoriesBug(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)

	scenario, err := o.populateCategories(benchSet)
	require.NoError(t, err)

	// Config has 1 category. With the bug, scenario.Categories has
	// 1 empty + 1 real = 2 entries. Without the bug, just 1.
	assert.Len(t, scenario.Categories, 1)
}

func TestScenarize(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	scenario, err := o.Scenarize(sets)
	require.NoError(t, err)

	require.NotNil(t, scenario)
	assert.Equal(t, "test-scenario", scenario.Name)

	// Filter out empty categories (due to the prepend bug)
	var nonEmpty int
	for _, cat := range scenario.Categories {
		if cat.ID != "" {
			nonEmpty++
		}
	}
	assert.Equal(t, 1, nonEmpty)
}

func TestScenarizeEnvironment(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	cfg.Environment = "test-env"
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	scenario, err := o.Scenarize(sets)
	require.NoError(t, err)

	for _, cat := range scenario.Categories {
		if cat.ID == "" {
			continue
		}
		assert.Equal(t, "test-env", cat.Environment)
	}
}

func TestScenarizeEmptySets(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	scenario, err := o.Scenarize(nil)
	require.NoError(t, err)
	require.NotNil(t, scenario)
}

func TestDefaultString(t *testing.T) {
	tests := []struct {
		in, def, want string
	}{
		{"value", "default", "value"},
		{"", "default", "default"},
		{"", "", ""},
		{"value", "", "value"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, defaultString(tt.in, tt.def))
	}
}

func TestParseBenchmarkNameEnvironment(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	parsed, ok := o.parseBenchmarkName("BenchmarkGreater/reflect/int-16", "file.txt", "linux amd64")
	require.True(t, ok)
	assert.Equal(t, "linux amd64", parsed.Environment)

	// Config environment takes precedence
	cfg.Environment = "override-env"
	parsed, ok = o.parseBenchmarkName("BenchmarkGreater/reflect/int-16", "file.txt", "linux amd64")
	require.True(t, ok)
	assert.Equal(t, "override-env", parsed.Environment)
}

func TestSeriesForPointNames(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)

	category := cfg.Categories[0]
	series := benchSet.SeriesFor(config.MetricNsPerOp, "reflect", category)

	require.NotEmpty(t, series)

	// Verify point names follow the pattern "function - version - context"
	for _, point := range series[0].Points {
		assert.NotEmpty(t, point.Name)
		// Name should contain function, version and context
		for _, part := range []string{"greater", "reflect"} {
			assert.Contains(t, point.Name, part)
		}
	}
}

func TestMultipleVersionSeries(t *testing.T) {
	cfg := mustLoadConfig(t, genericsConfig())
	o := New(cfg)

	sets := []parser.Set{buildGenericsSet()}
	benchSet, err := o.parseBenchmarks(sets)
	require.NoError(t, err)

	category := cfg.Categories[0]

	// Get series for both versions
	reflectSeries := benchSet.SeriesFor(config.MetricNsPerOp, "reflect", category)
	genericsSeries := benchSet.SeriesFor(config.MetricNsPerOp, "generics", category)

	assert.NotEmpty(t, reflectSeries)
	assert.NotEmpty(t, genericsSeries)

	// Generic benchmarks should have lower ns/op values in our test data
	if len(reflectSeries) > 0 && len(genericsSeries) > 0 &&
		len(reflectSeries[0].Points) > 0 && len(genericsSeries[0].Points) > 0 {
		if genericsSeries[0].Points[0].Value >= reflectSeries[0].Points[0].Value {
			t.Logf("Note: generic ns/op (%f) >= reflect ns/op (%f) - unexpected for test data",
				genericsSeries[0].Points[0].Value, reflectSeries[0].Points[0].Value)
		}
	}
}

// helpers

func mustLoadConfig(t *testing.T, yamlContent string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(yamlContent), 0o600))
	cfg, err := config.Load(file)
	require.NoError(t, err)
	return cfg
}

func buildGenericsSet() parser.Set {
	return parser.Set{
		Set: parse.Set{
			"BenchmarkGreater/reflect/int-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/reflect/int-16", N: 5000000, NsPerOp: 245.3, AllocedBytesPerOp: 64, AllocsPerOp: 2},
			},
			"BenchmarkGreater/generic/int-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/generic/int-16", N: 150000000, NsPerOp: 7.89, AllocedBytesPerOp: 0, AllocsPerOp: 0},
			},
			"BenchmarkGreater/reflect/float64-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/reflect/float64-16", N: 4500000, NsPerOp: 267.8, AllocedBytesPerOp: 64, AllocsPerOp: 2},
			},
			"BenchmarkGreater/generic/float64-16": []*parse.Benchmark{
				{Name: "BenchmarkGreater/generic/float64-16", N: 140000000, NsPerOp: 8.12, AllocedBytesPerOp: 0, AllocsPerOp: 0},
			},
		},
		File:        "test.json",
		Environment: "linux amd64 cpu: Test CPU",
	}
}

func genericsConfig() string {
	return `
name: test-scenario
metrics:
  - id: nsPerOp
    title: Benchmark Timings
    axis: 'ns/op'
  - id: allocsPerOp
    title: Benchmark Allocations
    axis: 'allocs/op'
functions:
  - id: greater
    title: Greater
    Match: 'GreaterT?'
    NotMatch: 'GreaterOr'
  - id: less
    title: Less
    Match: 'LessT?'
    NotMatch: 'LessOr'
  - id: negative
    title: Negative
    Match: 'NegativeT?'
contexts:
  - id: int
    Match: '/int'
  - id: float64
    Match: '/float64'
versions:
  - id: reflect
    Match: '/reflect/'
  - id: generics
    Match: '/generic/'
categories:
  - id: comparisons
    title: Comparisons
    includes:
      functions: [greater]
      versions: [reflect, generics]
      contexts: [int, float64]
      metrics: [nsPerOp, allocsPerOp]
`
}

func minimalConfig() string {
	return `
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
functions:
  - id: fn1
    Match: 'Bench'
categories:
  - id: cat1
    includes:
      functions: [fn1]
      metrics: [nsPerOp]
`
}

func configWithFileFallback() string {
	return `
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
functions:
  - id: greater
    title: Greater
    Match: 'Greater'
    NotMatch: 'GreaterOr'
contexts:
  - id: int
  - id: float64
versions:
  - id: reflect
  - id: generics
categories:
  - id: cat1
    includes:
      functions: [greater]
      metrics: [nsPerOp]
files:
  - id: benchfile
    MatchFile: 'bench_.*_test'
    contexts:
      - id: int
        Match: '_int_'
    versions:
      - id: reflect
        Match: '_reflect_'
      - id: generics
        Match: '_generics_'
`
}
