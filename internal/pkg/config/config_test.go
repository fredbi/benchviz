package config

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"go.yaml.in/yaml/v3"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestLoadDefault(t *testing.T) {
	cfg, err := loadDefaults()
	require.NoError(t, err)

	require.NoError(t, dumpConfig(os.Stdout, cfg))
}

func TestLoadDefaultContent(t *testing.T) {
	cfg, err := loadDefaults()
	require.NoError(t, err)

	// verify metrics are loaded
	assert.Len(t, cfg.Metrics, 4)

	// verify metric index is populated
	for _, name := range AllMetricNames() {
		_, ok := cfg.GetMetric(name)
		assert.True(t, ok, "expected metric %q in index", name)
	}

	// verify functions
	assert.Len(t, cfg.Functions, 5)

	for _, id := range []string{"greater", "less", "positive", "negative", "elements-match"} {
		_, ok := cfg.GetFunction(id)
		assert.True(t, ok, "expected function %q in index", id)
	}

	// verify contexts
	assert.Len(t, cfg.Contexts, 7)

	// verify versions
	assert.Len(t, cfg.Versions, 2)

	// verify categories
	assert.Len(t, cfg.Categories, 2)

	// verify rendering defaults
	assert.Equal(t, "roma", cfg.Render.Theme)
	assert.Equal(t, "barchart", cfg.Render.Chart)
	assert.Equal(t, ScaleAuto, cfg.Render.Scale)
	assert.Equal(t, 2, cfg.Render.Layout.Horizontal)
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	yamlContent := minimalValidYAML()

	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(yamlContent), 0o600))

	cfg, err := load(os.DirFS(dir), "config.yaml")
	require.NoError(t, err)

	assert.Len(t, cfg.Functions, 1)

	_, ok := cfg.GetFunction("fn1")
	assert.True(t, ok, "expected function fn1 in index")
}

func TestLoadAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(minimalValidYAML()), 0o600))

	cfg, err := Load(file)
	require.NoError(t, err)

	_, ok := cfg.GetFunction("fn1")
	assert.True(t, ok, "expected function fn1 in index")
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := load(os.DirFS(dir), "nonexistent.yaml")
	require.Error(t, err)
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(file, []byte(":\n  :\n    - [invalid"), 0o600))

	_, err := load(os.DirFS(dir), "bad.yaml")
	require.Error(t, err)
}

func TestMetricName(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		assert.Equal(t, "nsPerOp", MetricNsPerOp.String())
	})

	t.Run("IsValid", func(t *testing.T) {
		valid := []MetricName{MetricNsPerOp, MetricAllocsPerOp, MetricBytesPerOp, MetricMBPerS}
		for _, m := range valid {
			assert.True(t, m.IsValid(), "expected %q to be valid", m)
		}

		invalid := []MetricName{"unknown", "", "nsperop", "NS_PER_OP"}
		for _, m := range invalid {
			assert.False(t, m.IsValid(), "expected %q to be invalid", m)
		}
	})

	t.Run("AllMetricNames", func(t *testing.T) {
		names := AllMetricNames()
		require.Len(t, names, 4)
		for _, n := range names {
			assert.True(t, n.IsValid(), "AllMetricNames() returned invalid name %q", n)
		}
	})
}

func TestObjectMatchString(t *testing.T) {
	tests := []struct {
		name   string
		obj    Object
		input  string
		wantID string
		wantOk bool
	}{
		{
			name:   "both matchers nil returns false",
			obj:    Object{ID: "x"},
			input:  "anything",
			wantOk: false,
		},
		{
			name:   "match only, matches",
			obj:    mustObject("fn1", "Foo", ""),
			input:  "BenchmarkFoo",
			wantID: "fn1",
			wantOk: true,
		},
		{
			name:   "match only, no match",
			obj:    mustObject("fn1", "Foo", ""),
			input:  "BenchmarkBar",
			wantOk: false,
		},
		{
			name:   "notMatch only, not excluded",
			obj:    mustObject("fn1", "", "Exclude"),
			input:  "BenchmarkFoo",
			wantID: "fn1",
			wantOk: true,
		},
		{
			name:   "notMatch only, excluded",
			obj:    mustObject("fn1", "", "Exclude"),
			input:  "BenchmarkExclude",
			wantOk: false,
		},
		{
			name:   "match and notMatch, both match",
			obj:    mustObject("fn1", "Greater", "GreaterOr"),
			input:  "GreaterOrEqual",
			wantOk: false,
		},
		{
			name:   "match and notMatch, match only positive",
			obj:    mustObject("fn1", "Greater", "GreaterOr"),
			input:  "GreaterThan",
			wantID: "fn1",
			wantOk: true,
		},
		{
			name:   "match and notMatch, neither matches",
			obj:    mustObject("fn1", "Greater", "GreaterOr"),
			input:  "LessThan",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := tt.obj.MatchString(tt.input)
			assert.Equal(t, tt.wantOk, ok, "MatchString(%q) ok", tt.input)
			assert.Equal(t, tt.wantID, id, "MatchString(%q) id", tt.input)
		})
	}
}

func TestFileMatchString(t *testing.T) {
	t.Run("nil match returns false", func(t *testing.T) {
		f := File{ID: "f1"}
		_, ok := f.MatchString("anything")
		assert.False(t, ok)
	})

	t.Run("matching file", func(t *testing.T) {
		f := mustFile("f1", `bench_.*\.txt`)
		id, ok := f.MatchString("bench_results.txt")
		assert.True(t, ok, "expected match")
		assert.Equal(t, "f1", id)
	})

	t.Run("non-matching file", func(t *testing.T) {
		f := mustFile("f1", `bench_.*\.txt`)
		_, ok := f.MatchString("results.csv")
		assert.False(t, ok)
	})
}

func TestFindFunction(t *testing.T) {
	cfg := mustLoadDefaults(t)

	tests := []struct {
		input  string
		wantID string
		wantOk bool
	}{
		{"BenchmarkGreaterThan", "greater", true},
		{"BenchmarkGreater", "greater", true},
		{"BenchmarkGreaterOrEqual", "", false}, // excluded by NotMatch
		{"BenchmarkLessThan", "less", true},
		{"BenchmarkLessOrEqual", "", false}, // excluded by NotMatch
		{"BenchmarkNegative", "negative", true},
		{"BenchmarkElementsMatchT", "elements-match", true},
		{"BenchmarkUnknown", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			id, ok := cfg.FindFunction(tt.input)
			assert.Equal(t, tt.wantOk, ok, "FindFunction(%q) ok", tt.input)
			assert.Equal(t, tt.wantID, id, "FindFunction(%q) id", tt.input)
		})
	}
}

func TestFindVersion(t *testing.T) {
	cfg := mustLoadTestConfig(t, configWithVersionMatchers())

	tests := []struct {
		input  string
		wantID string
		wantOk bool
	}{
		{"reflect-based", "reflect", true},
		{"generics-based", "generics", true},
		{"unknown", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			id, ok := cfg.FindVersion(tt.input)
			assert.Equal(t, tt.wantOk, ok, "FindVersion(%q) ok", tt.input)
			assert.Equal(t, tt.wantID, id, "FindVersion(%q) id", tt.input)
		})
	}
}

func TestFindContext(t *testing.T) {
	cfg := mustLoadTestConfig(t, configWithContextMatchers())

	tests := []struct {
		input  string
		wantID string
		wantOk bool
	}{
		{"small-input", "small", true},
		{"large-input", "large", true},
		{"unknown", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			id, ok := cfg.FindContext(tt.input)
			assert.Equal(t, tt.wantOk, ok, "FindContext(%q) ok", tt.input)
			assert.Equal(t, tt.wantID, id, "FindContext(%q) id", tt.input)
		})
	}
}

func TestFindVersionFromFile(t *testing.T) {
	cfg := mustLoadTestConfig(t, configWithFiles())

	tests := []struct {
		input  string
		wantID string
		wantOk bool
	}{
		{"bench_reflect_test.txt", "reflect", true},
		{"bench_generics_test.txt", "generics", true},
		{"bench_unknown_test.txt", "", false}, // file matches but no version match
		{"other.txt", "", false},              // file doesn't match
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			id, ok := cfg.FindVersionFromFile(tt.input)
			assert.Equal(t, tt.wantOk, ok, "FindVersionFromFile(%q) ok", tt.input)
			assert.Equal(t, tt.wantID, id, "FindVersionFromFile(%q) id", tt.input)
		})
	}
}

func TestFindContextFromFile(t *testing.T) {
	cfg := mustLoadTestConfig(t, configWithFiles())

	tests := []struct {
		input  string
		wantID string
		wantOk bool
	}{
		{"bench_int_test.txt", "int", true},
		{"bench_float64_test.txt", "", false}, // file matches but no context match
		{"other.txt", "", false},              // file doesn't match
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			id, ok := cfg.FindContextFromFile(tt.input)
			assert.Equal(t, tt.wantOk, ok, "FindContextFromFile(%q) ok", tt.input)
			assert.Equal(t, tt.wantID, id, "FindContextFromFile(%q) id", tt.input)
		})
	}
}

func TestGetters(t *testing.T) {
	cfg := mustLoadDefaults(t)

	t.Run("GetFunction found", func(t *testing.T) {
		fn, ok := cfg.GetFunction("greater")
		require.True(t, ok, "expected to find function 'greater'")
		assert.Equal(t, "Greater", fn.Title)
	})

	t.Run("GetFunction not found", func(t *testing.T) {
		_, ok := cfg.GetFunction("nonexistent")
		assert.False(t, ok)
	})

	t.Run("GetContext found", func(t *testing.T) {
		ctx, ok := cfg.GetContext("int")
		require.True(t, ok, "expected to find context 'int'")
		assert.Equal(t, "Int", ctx.Title)
	})

	t.Run("GetContext not found", func(t *testing.T) {
		_, ok := cfg.GetContext("nonexistent")
		assert.False(t, ok)
	})

	t.Run("GetVersion found", func(t *testing.T) {
		v, ok := cfg.GetVersion("reflect")
		require.True(t, ok, "expected to find version 'reflect'")
		assert.Equal(t, "reflect", v.Title)
	})

	t.Run("GetVersion not found", func(t *testing.T) {
		_, ok := cfg.GetVersion("nonexistent")
		assert.False(t, ok)
	})

	t.Run("GetMetric found", func(t *testing.T) {
		m, ok := cfg.GetMetric(MetricNsPerOp)
		require.True(t, ok, "expected to find metric 'nsPerOp'")
		assert.Equal(t, "Benchmark Timings", m.Title)
		assert.Equal(t, "ns/op", m.Axis)
	})

	t.Run("GetMetric not found", func(t *testing.T) {
		_, ok := cfg.GetMetric("invalid")
		assert.False(t, ok)
	})
}

func TestValidationEmptyID(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "function with empty ID",
			yaml: `
metrics:
  - id: nsPerOp
functions:
  - id: ""
    Match: "foo"
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
`,
		},
		{
			name: "context with empty ID",
			yaml: `
metrics:
  - id: nsPerOp
contexts:
  - id: ""
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
`,
		},
		{
			name: "version with empty ID",
			yaml: `
metrics:
  - id: nsPerOp
versions:
  - id: ""
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
`,
		},
		{
			name: "metric with empty ID",
			yaml: `
metrics:
  - id: ""
categories:
  - id: cat1
    includes:
      metrics: [""]
`,
		},
		{
			name: "category with empty ID",
			yaml: `
metrics:
  - id: nsPerOp
categories:
  - id: ""
    includes:
      metrics: [nsPerOp]
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadFromString(t, tt.yaml)
			require.Error(t, err)
		})
	}
}

func TestValidationDuplicateID(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "duplicate function ID",
			yaml: `
metrics:
  - id: nsPerOp
functions:
  - id: fn1
    Match: "Foo"
  - id: fn1
    Match: "Bar"
categories:
  - id: cat1
    includes:
      functions: [fn1]
      metrics: [nsPerOp]
`,
		},
		{
			name: "duplicate context ID",
			yaml: `
metrics:
  - id: nsPerOp
contexts:
  - id: ctx1
  - id: ctx1
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
`,
		},
		{
			name: "duplicate version ID",
			yaml: `
metrics:
  - id: nsPerOp
versions:
  - id: v1
  - id: v1
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
`,
		},
		{
			name: "duplicate metric ID",
			yaml: `
metrics:
  - id: nsPerOp
  - id: nsPerOp
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadFromString(t, tt.yaml)
			require.Error(t, err)
		})
	}
}

func TestValidationInvalidMetricName(t *testing.T) {
	yamlContent := `
metrics:
  - id: invalidMetricName
categories:
  - id: cat1
    includes:
      metrics: [invalidMetricName]
`
	_, err := loadFromString(t, yamlContent)
	require.Error(t, err)
}

func TestValidationCategoryReferences(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "category references unknown function",
			yaml: `
metrics:
  - id: nsPerOp
functions:
  - id: fn1
    Match: "Foo"
categories:
  - id: cat1
    includes:
      functions: [unknown]
      metrics: [nsPerOp]
`,
		},
		{
			name: "category references unknown context",
			yaml: `
metrics:
  - id: nsPerOp
contexts:
  - id: ctx1
categories:
  - id: cat1
    includes:
      contexts: [unknown]
      metrics: [nsPerOp]
`,
		},
		{
			name: "category references unknown version",
			yaml: `
metrics:
  - id: nsPerOp
versions:
  - id: v1
categories:
  - id: cat1
    includes:
      versions: [unknown]
      metrics: [nsPerOp]
`,
		},
		{
			name: "category references unknown metric",
			yaml: `
metrics:
  - id: nsPerOp
categories:
  - id: cat1
    includes:
      metrics: [allocsPerOp]
`,
		},
		{
			name: "category with no metrics",
			yaml: `
metrics:
  - id: nsPerOp
categories:
  - id: cat1
    includes:
      functions: []
`,
		},
		{
			name: "category with more than 2 metrics",
			yaml: `
metrics:
  - id: nsPerOp
  - id: allocsPerOp
  - id: bytesPerOp
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp, allocsPerOp, bytesPerOp]
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadFromString(t, tt.yaml)
			require.Error(t, err)
		})
	}
}

// TestValidationCategoryDefaultIncludes verifies that when a category
// doesn't specify functions/contexts/versions, all defined ones are injected.
func TestValidationCategoryDefaultIncludes(t *testing.T) {
	yamlContent := `
metrics:
  - id: nsPerOp
functions:
  - id: fn1
    Match: "Foo"
  - id: fn2
    Match: "Bar"
contexts:
  - id: ctx1
  - id: ctx2
versions:
  - id: v1
  - id: v2
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
`
	cfg, err := loadFromString(t, yamlContent)
	require.NoError(t, err)

	cat := cfg.Categories[0]

	assert.Len(t, cat.Includes.Functions, 2)
	assert.Len(t, cat.Includes.Contexts, 2)
	assert.Len(t, cat.Includes.Versions, 2)
}

func TestValidationInvalidRegexp(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "invalid function match regexp",
			yaml: `
metrics:
  - id: nsPerOp
functions:
  - id: fn1
    Match: "[invalid"
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
`,
		},
		{
			name: "invalid function notMatch regexp",
			yaml: `
metrics:
  - id: nsPerOp
functions:
  - id: fn1
    Match: "valid"
    NotMatch: "[invalid"
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
`,
		},
		{
			name: "invalid file matchFile regexp",
			yaml: `
metrics:
  - id: nsPerOp
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
files:
  - id: f1
    MatchFile: "[invalid"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadFromString(t, tt.yaml)
			require.Error(t, err)
		})
	}
}

func TestValidationFileReferences(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "file with missing ID",
			yaml: `
metrics:
  - id: nsPerOp
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
files:
  - id: ""
    MatchFile: "bench_.*"
`,
		},
		{
			name: "file references unknown context",
			yaml: `
metrics:
  - id: nsPerOp
contexts:
  - id: ctx1
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
files:
  - id: f1
    MatchFile: "bench_.*"
    contexts:
      - id: unknown
        Match: "foo"
`,
		},
		{
			name: "file references unknown version",
			yaml: `
metrics:
  - id: nsPerOp
versions:
  - id: v1
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
files:
  - id: f1
    MatchFile: "bench_.*"
    versions:
      - id: unknown
        Match: "foo"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadFromString(t, tt.yaml)
			require.Error(t, err)
		})
	}
}

func TestTitleize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "Hello"},
		{"hello-world", "Hello World"},
		{"hello_world", "Hello World"},
		{"int", "Int"},
		{"int-small", "Int Small"},
		{"elements-match", "Elements Match"},
		{"nsPerOp", "NsPerOp"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, titleize(tt.input))
		})
	}
}

func TestTitleizeMetricName(t *testing.T) {
	assert.Equal(t, "NsPerOp", titleize(MetricNsPerOp))
}

func TestAutoTitle(t *testing.T) {
	// Contexts without explicit title get auto-titled
	cfg := mustLoadDefaults(t)
	ctx, ok := cfg.GetContext("int")
	require.True(t, ok, "expected context 'int'")
	assert.Equal(t, "Int", ctx.Title)

	// Context with explicit title keeps it
	ctx, ok = cfg.GetContext("int-small")
	require.True(t, ok, "expected context 'int-small'")
	assert.Equal(t, "int - small", ctx.Title)
}

// helpers

func dumpConfig(w io.Writer, cfg *Config) error {
	var raw any
	err := mapstructure.Decode(cfg, &raw)
	if err != nil {
		return err
	}

	enc := yaml.NewEncoder(w)

	return enc.Encode(raw)
}

func mustLoadDefaults(t *testing.T) *Config {
	t.Helper()
	cfg, err := loadDefaults()
	require.NoError(t, err)
	return cfg
}

func loadFromString(t *testing.T, yamlContent string) (*Config, error) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(yamlContent), 0o600))
	return load(os.DirFS(dir), "config.yaml")
}

func mustLoadTestConfig(t *testing.T, yamlContent string) *Config {
	t.Helper()
	cfg, err := loadFromString(t, yamlContent)
	require.NoError(t, err)
	return cfg
}

func mustObject(id, match, notMatch string) Object { //nolint:unparam // id maintained for future test extensions
	o := Object{ID: id, Match: match, NotMatch: notMatch}
	m, nm, err := compileRex(o)
	if err != nil {
		panic(err)
	}
	o.match = m
	o.notMatch = nm
	return o
}

func mustFile(id, matchFile string) File {
	f := File{ID: id, MatchFile: matchFile}
	if matchFile != "" {
		m, _, err := compileRex(Object{Match: matchFile})
		if err != nil {
			panic(err)
		}
		f.match = m
	}
	return f
}

func minimalValidYAML() string {
	return `
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
functions:
  - id: fn1
    Match: "Bench"
categories:
  - id: cat1
    includes:
      functions: [fn1]
      metrics: [nsPerOp]
`
}

func configWithVersionMatchers() string {
	return `
metrics:
  - id: nsPerOp
versions:
  - id: reflect
    Match: "reflect"
  - id: generics
    Match: "generics"
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
`
}

func configWithContextMatchers() string {
	return `
metrics:
  - id: nsPerOp
contexts:
  - id: small
    Match: "small"
  - id: large
    Match: "large"
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
`
}

func configWithFiles() string {
	return `
metrics:
  - id: nsPerOp
contexts:
  - id: int
  - id: float64
versions:
  - id: reflect
  - id: generics
categories:
  - id: cat1
    includes:
      metrics: [nsPerOp]
files:
  - id: benchfile
    MatchFile: "bench_.*_test\\.txt"
    contexts:
      - id: int
        Match: "int"
    versions:
      - id: reflect
        Match: "reflect"
      - id: generics
        Match: "generics"
`
}
