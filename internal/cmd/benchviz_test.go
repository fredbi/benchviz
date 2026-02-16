package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fredbi/benchviz/internal/pkg/config"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestNewCommand(t *testing.T) {
	cli := NewCommand()
	require.NotNil(t, cli)
	assert.NotNil(t, cli.L)
	// Verify defaults from registerFlags
	assert.Equal(t, "benchviz.yaml", cli.Config)
	assert.Equal(t, "-", cli.OutputFile)
}

func TestInferHTMLFile(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"output.png", "output.html"},
		{"output.html", "output.html"},
		{"output", "output.html"},
		{"path/to/output.png", "path/to/output.html"},
		{"output.svg", "output.html"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, inferHTMLFile(tt.input))
		})
	}
}

func TestInferImageFile(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"output.html", "output.png"},
		{"output.png", "output.png"},
		{"output", "output.png"},
		{"path/to/output.html", "path/to/output.png"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, inferImageFile(tt.input))
		})
	}
}

func TestSetConfigJSON(t *testing.T) {
	cfg := &config.Config{}
	cli := &Command{
		IsJSON:      true,
		Environment: "test-env",
		L:           newTestLogger(),
	}

	require.NoError(t, cli.setConfig(cfg))

	assert.True(t, cfg.IsJSON)
	assert.Equal(t, "test-env", cfg.Environment)
}

func TestSetConfigOutputToStdout(t *testing.T) {
	cfg := &config.Config{}
	cli := &Command{
		OutputFile: "-",
		L:          newTestLogger(),
	}

	require.NoError(t, cli.setConfig(cfg))

	// When no output file specified, HTML goes to stdout
	assert.Equal(t, "-", cfg.Outputs.HTMLFile)
}

func TestSetConfigOutputFile(t *testing.T) {
	cfg := &config.Config{}
	cli := &Command{
		OutputFile: "results.png",
		L:          newTestLogger(),
	}

	require.NoError(t, cli.setConfig(cfg))

	assert.Equal(t, "results.html", cfg.Outputs.HTMLFile)
}

func TestSetConfigOutputFileWithPng(t *testing.T) {
	cfg := &config.Config{
		Outputs: config.Output{
			PngFile: "existing.png",
		},
	}
	cli := &Command{
		OutputFile: "results.html",
		Png:        true,
		L:          newTestLogger(),
	}

	require.NoError(t, cli.setConfig(cfg))

	assert.Equal(t, "results.html", cfg.Outputs.HTMLFile)
	assert.Equal(t, "results.png", cfg.Outputs.PngFile)
}

func TestSetConfigTempHTML(t *testing.T) {
	cfg := &config.Config{
		Outputs: config.Output{
			PngFile: "output.png",
		},
	}
	cli := &Command{
		L: newTestLogger(),
	}

	require.NoError(t, cli.setConfig(cfg))

	assert.True(t, cfg.Outputs.IsTemp)
	assert.NotEmpty(t, cfg.Outputs.HTMLFile)
	assert.True(t, strings.Contains(cfg.Outputs.HTMLFile, "benchviz"),
		"expected temp file name to contain 'benchviz', got %q", cfg.Outputs.HTMLFile)

	// Clean up temp file
	os.Remove(cfg.Outputs.HTMLFile)
}

func TestPrepareConfig(t *testing.T) {
	cfgFile := writeTestConfig(t, testConfig())

	cli := &Command{
		Config: cfgFile,
		L:      newTestLogger(),
	}

	cfg, cleanup, err := cli.prepareConfig()
	require.NoError(t, err)
	defer cleanup()

	require.NotNil(t, cfg)
}

func TestPrepareConfigMissingFile(t *testing.T) {
	cli := &Command{
		Config: "/nonexistent/config.yaml",
		L:      newTestLogger(),
	}

	_, cleanup, err := cli.prepareConfig()
	require.Error(t, err)
	assert.Nil(t, cleanup)
}

func TestPrepareConfigDefaultArgs(t *testing.T) {
	cfgFile := writeTestConfig(t, testConfig())

	cli := &Command{
		Config: cfgFile,
		L:      newTestLogger(),
	}

	// When args is empty, "-" (stdin) should be appended
	cfg, cleanup, err := cli.prepareConfig()
	require.NoError(t, err)
	defer cleanup()

	require.NotNil(t, cfg)
}

func TestBuildPage(t *testing.T) {
	cfg := mustLoadTestConfig(t, testConfig())

	page, err := buildPage(cfg, []string{parserTestdataPath("sample_generics.json")})
	require.NoError(t, err)
	require.NotNil(t, page)
}

func TestBuildPageMissingFile(t *testing.T) {
	cfg := mustLoadTestConfig(t, testConfig())

	_, err := buildPage(cfg, []string{"/nonexistent/file.txt"})
	require.Error(t, err)
}

func TestExecuteHTMLOutput(t *testing.T) {
	cfgFile := writeTestConfig(t, testConfig())
	outFile := filepath.Join(t.TempDir(), "output.html")

	cli := &Command{
		Config:     cfgFile,
		IsJSON:     true,
		OutputFile: outFile,
		L:          newTestLogger(),
	}

	require.NoError(t, cli.Execute(parserTestdataPath("sample_generics.json")))

	// Verify HTML file was created
	info, err := os.Stat(outFile)
	require.NoError(t, err)
	assert.NotZero(t, info.Size())
}

func TestExecuteMultipleInputs(t *testing.T) {
	cfgFile := writeTestConfig(t, testConfigText())
	outFile := filepath.Join(t.TempDir(), "output.html")

	cli := &Command{
		Config:     cfgFile,
		OutputFile: outFile,
		L:          newTestLogger(),
	}

	require.NoError(t, cli.Execute(
		parserTestdataPath("run.txt"),
		parserTestdataPath("run1.txt"),
	))

	info, err := os.Stat(outFile)
	require.NoError(t, err)
	assert.NotZero(t, info.Size())
}

func TestExecuteMissingInput(t *testing.T) {
	cfgFile := writeTestConfig(t, testConfig())

	cli := &Command{
		Config:     cfgFile,
		OutputFile: filepath.Join(t.TempDir(), "output.html"),
		L:          newTestLogger(),
	}

	require.Error(t, cli.Execute("/nonexistent/file.txt"))
}

func TestGenerateConfigJSON(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "generated.yaml")

	cli := &Command{
		Config:         outFile,
		IsJSON:         true,
		GenerateConfig: true,
		L:              newTestLogger(),
	}

	require.NoError(t, cli.Execute(parserTestdataPath("sample_generics.json")))

	// Verify the file was created
	info, err := os.Stat(outFile)
	require.NoError(t, err)
	assert.NotZero(t, info.Size())

	// Verify it loads as a valid config
	cfg, err := config.Load(outFile)
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.Functions)
	assert.NotEmpty(t, cfg.Metrics)
	assert.NotEmpty(t, cfg.Categories)
}

func TestGenerateConfigText(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "generated.yaml")

	cli := &Command{
		Config:         outFile,
		GenerateConfig: true,
		L:              newTestLogger(),
	}

	require.NoError(t, cli.Execute(
		parserTestdataPath("run.txt"),
		parserTestdataPath("run1.txt"),
	))

	info, err := os.Stat(outFile)
	require.NoError(t, err)
	assert.NotZero(t, info.Size())

	cfg, err := config.Load(outFile)
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.Functions)
	assert.NotEmpty(t, cfg.Metrics)
}

func TestGenerateConfigMissingInput(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "generated.yaml")

	cli := &Command{
		Config:         outFile,
		GenerateConfig: true,
		L:              newTestLogger(),
	}

	require.Error(t, cli.Execute("/nonexistent/file.txt"))
}

// helpers

func newTestLogger() *slog.Logger {
	return slog.Default().With(slog.String("module", "test"))
}

func writeTestConfig(t *testing.T, yamlContent string) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(yamlContent), 0o600))
	return file
}

func mustLoadTestConfig(t *testing.T, yamlContent string) *config.Config {
	t.Helper()
	file := writeTestConfig(t, yamlContent)
	cfg, err := config.Load(file)
	require.NoError(t, err)
	return cfg
}

func parserTestdataPath(name string) string {
	return filepath.Join("..", "pkg", "parser", "testdata", name)
}

func testConfig() string {
	return `
name: Test
render:
  theme: roma
  legend: bottom
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
  - id: allocsPerOp
    title: Allocations
    axis: 'allocs/op'
functions:
  - id: greater
    Match: 'Greater'
    NotMatch: 'GreaterOr'
  - id: less
    Match: 'Less'
    NotMatch: 'LessOr'
  - id: positive
    Match: 'Positive'
  - id: negative
    Match: 'Negative'
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
    includes:
      metrics: [nsPerOp, allocsPerOp]
`
}

func testConfigText() string {
	return `
name: Text Test
render:
  theme: roma
  legend: bottom
metrics:
  - id: nsPerOp
    title: Timings
    axis: 'ns/op'
functions:
  - id: readjson
    Match: 'ReadJSON'
  - id: writejson
    Match: 'WriteJSON'
contexts:
  - id: small
    Match: '_small'
  - id: medium
    Match: '_medium'
  - id: large
    Match: '_large'
versions:
  - id: stdlib
    Match: 'standard'
  - id: easyjson
    Match: 'easyjson'
categories:
  - id: json
    includes:
      metrics: [nsPerOp]
`
}
