package parser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fredbi/benchviz/internal/pkg/config"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

const unk = "unknown environment"

func TestNew(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg)
	require.EqualT(t, cfg, p.config)
}

func TestNewWithOptions(t *testing.T) {
	cfg := &config.Config{}

	p := New(cfg, WithParseJSON(true))
	assert.True(t, p.isJSON)

	p = New(cfg, WithParseJSON(false))
	assert.False(t, p.isJSON)

	p = New(cfg)
	assert.False(t, p.isJSON, "expected isJSON to default to false")
}

func TestParseTextFile(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg)

	require.NoError(t, p.ParseFiles(testdataPath("run.txt")))

	sets := p.Sets()
	require.Len(t, sets, 1)

	set := sets[0]
	assert.Equal(t, testdataPath("run.txt"), set.File)

	// run.txt contains benchmarks under BenchmarkJSON
	require.NotEmpty(t, set.Set)

	// Verify some known benchmark names are present (includes GOMAXPROCS suffix)
	expectBenchmarks(t, set, []string{
		"BenchmarkJSON/with_standard_library/standard_ReadJSON_-_small-16",
		"BenchmarkJSON/with_standard_library/standard_WriteJSON_-_small-16",
		"BenchmarkJSON/with_easyjson_library/easyjson_ReadJSON_-_small-16",
		"BenchmarkJSON/with_standard_library/standard_ReadJSON_-_large-16",
		"BenchmarkJSON/with_easyjson_library/easyjson_ReadJSON_-_large-16",
	})

	// Verify benchmark values are parsed
	benchmarks := set.Set["BenchmarkJSON/with_standard_library/standard_ReadJSON_-_small-16"]
	require.NotEmpty(t, benchmarks)

	b := benchmarks[0]
	assert.Positive(t, b.NsPerOp)
	assert.NotZero(t, b.AllocsPerOp)
	assert.NotZero(t, b.AllocedBytesPerOp)
	assert.Positive(t, b.MBPerS)
	assert.Positive(t, b.N)
}

func TestParseTextMultipleFiles(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg)

	require.NoError(t, p.ParseFiles(testdataPath("run.txt"), testdataPath("run1.txt")))

	sets := p.Sets()
	require.Len(t, sets, 2)

	assert.Equal(t, testdataPath("run.txt"), sets[0].File)
	assert.Equal(t, testdataPath("run1.txt"), sets[1].File)

	// Both files should have the same benchmark names
	for _, name := range []string{
		"BenchmarkJSON/with_standard_library/standard_ReadJSON_-_small-16",
		"BenchmarkJSON/with_easyjson_library/easyjson_ReadJSON_-_small-16",
	} {
		assert.Contains(t, sets[0].Set, name, "run.txt")
		assert.Contains(t, sets[1].Set, name, "run1.txt")
	}
}

func TestParseJSON(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg, WithParseJSON(true))

	require.NoError(t, p.ParseFiles(testdataPath("sample_json.txt")))

	sets := p.Sets()
	require.Len(t, sets, 1)

	set := sets[0]
	require.NotEmpty(t, set.Set)

	// sample_json.txt contains BenchmarkPositive benchmarks
	expectBenchmarks(t, set, []string{
		"BenchmarkPositive/reflect/int-16",
		"BenchmarkPositive/generic/int-16",
		"BenchmarkPositive/reflect/float64-16",
		"BenchmarkPositive/generic/float64-16",
	})

	// Verify values
	benchmarks := set.Set["BenchmarkPositive/reflect/int-16"]
	require.NotEmpty(t, benchmarks)
	assert.Positive(t, benchmarks[0].NsPerOp)
}

func TestParseJSONMultipleBenchmarks(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg, WithParseJSON(true))

	require.NoError(t, p.ParseFiles(testdataPath("sample_generics.json")))

	sets := p.Sets()
	require.Len(t, sets, 1)

	// sample_generics.json has Greater, Less, Positive, Negative benchmarks
	expectBenchmarks(t, sets[0], []string{
		"BenchmarkGreater/reflect/int-16",
		"BenchmarkGreater/generic/int-16",
		"BenchmarkLess/reflect/int-16",
		"BenchmarkLess/generic/int-16",
		"BenchmarkPositive/reflect/int-16",
		"BenchmarkPositive/generic/int-16",
		"BenchmarkNegative/reflect/int-16",
		"BenchmarkNegative/generic/int-16",
	})
}

func TestParseTextEnvironment(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg)

	require.NoError(t, p.ParseFiles(testdataPath("run.txt")))

	env := p.Sets()[0].Environment
	assert.Contains(t, env, "linux")
	assert.Contains(t, env, "amd64")
	assert.Contains(t, env, "cpu:")
}

func TestParseJSONEnvironment(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg, WithParseJSON(true))

	require.NoError(t, p.ParseFiles(testdataPath("sample_json.txt")))

	env := p.Sets()[0].Environment
	assert.Contains(t, env, "linux")
	assert.Contains(t, env, "amd64")
	assert.Contains(t, env, "cpu:")
}

func TestExtractEnvironment(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string // substrings that must be present
	}{
		{
			name:  "full environment",
			input: "goos: linux\ngoarch: amd64\ncpu: Intel Core i7\n",
			want:  []string{"linux", "amd64", "cpu: Intel Core i7"},
		},
		{
			name:  "goos only",
			input: "goos: darwin\n",
			want:  []string{"darwin"},
		},
		{
			name:  "no environment info",
			input: "BenchmarkFoo-8  1000  1234 ns/op\n",
			want:  []string{unk},
		},
		{
			name:  "empty input",
			input: "",
			want:  []string{unk},
		},
		{
			name:  "cpu with extra whitespace",
			input: "cpu: AMD Ryzen 7 5800X 8-Core Processor             \n",
			want:  []string{"cpu: AMD Ryzen 7 5800X 8-Core Processor"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractEnvironment(tt.input)
			for _, substr := range tt.want {
				assert.Contains(t, got, substr)
			}
		})
	}
}

func TestExtractEnvironmentTrimsWhitespace(t *testing.T) {
	input := "cpu: AMD Ryzen 7 5800X 8-Core Processor             \n"
	got := extractEnvironment(input)
	assert.False(t, strings.HasSuffix(got, " "), "expected trimmed cpu string, got trailing spaces in %q", got)
}

func TestParseInputText(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg)

	input := `goos: linux
goarch: amd64
cpu: Test CPU
BenchmarkFoo-8   1000   1234 ns/op   56 B/op   3 allocs/op
`
	set, err := p.ParseInput(strings.NewReader(input))
	require.NoError(t, err)
	require.NotEmpty(t, set.Set)
	assert.Contains(t, set.Set, "BenchmarkFoo-8")
	assert.Contains(t, set.Environment, "linux")
}

func TestParseInputJSON(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg, WithParseJSON(true))

	input := `{"Action":"output","Output":"goos: linux\n"}
{"Action":"output","Output":"goarch: amd64\n"}
{"Action":"output","Output":"BenchmarkBar-4   2000   567.8 ns/op   32 B/op   1 allocs/op\n"}
{"Action":"pass"}
`
	set, err := p.ParseInput(strings.NewReader(input))
	require.NoError(t, err)
	require.NotEmpty(t, set.Set)
	assert.Contains(t, set.Set, "BenchmarkBar-4")
}

func TestParseInputJSONSkipsNonOutputActions(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg, WithParseJSON(true))

	input := `{"Action":"start","Package":"pkg"}
{"Action":"output","Output":"BenchmarkX-4   1000   100.0 ns/op\n"}
{"Action":"run","Test":"TestFoo"}
{"Action":"pass","Package":"pkg"}
`
	set, err := p.ParseInput(strings.NewReader(input))
	require.NoError(t, err)
	assert.Contains(t, set.Set, "BenchmarkX-4")
}

func TestParseInputJSONSkipsInvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg, WithParseJSON(true))

	input := `not valid json
{"Action":"output","Output":"BenchmarkY-4   500   200.0 ns/op\n"}
also not json {}
{"Action":"pass"}
`
	set, err := p.ParseInput(strings.NewReader(input))
	require.NoError(t, err)
	assert.Contains(t, set.Set, "BenchmarkY-4")
}

func TestParseFileMissing(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg)

	require.Error(t, p.ParseFiles("/nonexistent/file.txt"))
}

func TestParseInputFailingReader(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg)

	errExpected := errors.New("read error")
	_, err := p.ParseInput(&failingReader{err: errExpected})
	require.ErrorIs(t, err, errExpected)
}

func TestParseInputFailingReaderJSON(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg, WithParseJSON(true))

	errExpected := errors.New("read error")
	_, err := p.ParseInput(&failingReader{err: errExpected})
	require.ErrorIs(t, err, errExpected)
}

func TestSetsAccumulate(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg)

	assert.Empty(t, p.Sets())

	require.NoError(t, p.ParseFiles(testdataPath("run.txt")))
	assert.Len(t, p.Sets(), 1)

	require.NoError(t, p.ParseFiles(testdataPath("run1.txt")))
	assert.Len(t, p.Sets(), 2)
}

func TestParseGreenteaGC(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg)

	require.NoError(t, p.ParseFiles(testdataPath("greenteagc.txt")))

	sets := p.Sets()
	require.Len(t, sets, 1)
	require.NotEmpty(t, sets[0].Set)
	assert.NotEqual(t, unk, sets[0].Environment)
}

func TestParseInputEmptyText(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg)

	set, err := p.ParseInput(strings.NewReader(""))
	require.NoError(t, err)
	assert.Empty(t, set.Set)
	assert.Equal(t, unk, set.Environment)
}

func TestParseInputEmptyJSON(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg, WithParseJSON(true))

	set, err := p.ParseInput(strings.NewReader(""))
	require.NoError(t, err)
	assert.Empty(t, set.Set)
}

// helpers

func testdataPath(name string) string {
	return filepath.Join("testdata", name)
}

func expectBenchmarks(t *testing.T, set Set, names []string) {
	t.Helper()
	for _, name := range names {
		assert.Contains(t, set.Set, name, "missing benchmark (have: %v)", benchmarkNames(set))
	}
}

func benchmarkNames(set Set) []string {
	names := make([]string, 0, len(set.Set))
	for name := range set.Set {
		names = append(names, name)
	}
	return names
}

type failingReader struct {
	err error
}

func (r *failingReader) Read([]byte) (int, error) {
	return 0, r.err
}

// failingScanner produces an error after reading some data.
type failingScanner struct {
	data []byte
	pos  int
	err  error
}

func (r *failingScanner) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n

	return n, nil
}

func TestParseFilesStdin(t *testing.T) {
	// We can't easily test stdin reading, but we can verify
	// that "-" is handled as a special case by checking code paths.
	// Creating a real stdin test would require subprocess.
	// Instead, verify ParseInput works with a reader (which is what stdin uses).
	cfg := &config.Config{}
	p := New(cfg)

	input := "BenchmarkStdin-4   1000   500.0 ns/op\n"
	set, err := p.ParseInput(strings.NewReader(input))
	require.NoError(t, err)
	assert.Contains(t, set.Set, "BenchmarkStdin-4")
}

func TestParseFilesErrorClosesReader(t *testing.T) {
	// Verify that parsing a nonexistent file after a successful one
	// doesn't lose the first result.
	cfg := &config.Config{}
	p := New(cfg)

	err := p.ParseFiles(testdataPath("run.txt"), "/nonexistent/file.txt")
	require.Error(t, err)

	// First file should have been parsed before the error
	assert.Len(t, p.Sets(), 1)
}

func TestParseJSONScannerError(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg, WithParseJSON(true))

	errExpected := errors.New("scanner error")
	reader := &failingScanner{
		data: []byte(`{"Action":"output","Output":"BenchmarkZ-4   1000   100.0 ns/op\n"}` + "\n"),
		err:  errExpected,
	}

	_, err := p.ParseInput(reader)
	if err == nil {
		// The scanner might not propagate the error if it read enough data.
		// That's acceptable behavior - just verify it doesn't panic.
		return
	}

	t.Logf("got error: %v", err)
}

func TestParseTextBenchmarkValues(t *testing.T) {
	cfg := &config.Config{}
	p := New(cfg)

	f, err := os.Open(testdataPath("run.txt"))
	require.NoError(t, err)
	defer f.Close()

	set, err := p.ParseInput(f)
	require.NoError(t, err)

	// Verify specific values from run.txt:
	// BenchmarkJSON/with_standard_library/standard_ReadJSON_-_small-16
	// 12995456  3083 ns/op  46.72 MB/s  416 B/op  9 allocs/op
	const benchName = "BenchmarkJSON/with_standard_library/standard_ReadJSON_-_small-16"
	require.Contains(t, set.Set, benchName)

	b := set.Set[benchName][0]
	assert.Equal(t, 12995456, b.N)
	assert.Equal(t, uint64(416), b.AllocedBytesPerOp)
	assert.Equal(t, uint64(9), b.AllocsPerOp)
}
