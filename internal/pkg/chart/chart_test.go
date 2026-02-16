package chart

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fredbi/benchviz/internal/pkg/config"
	"github.com/fredbi/benchviz/internal/pkg/organizer"
	"github.com/fredbi/benchviz/internal/pkg/parser"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

// TestSmokeRenderFromTestdata is an end-to-end smoke test that loads
// benchmark data from parser testdata, organizes it, builds charts,
// and renders HTML output.
func TestSmokeRenderFromTestdata(t *testing.T) {
	cfg := mustLoadConfig(t, smokeConfig())

	// Parse benchmark data from testdata
	p := parser.New(cfg, parser.WithParseJSON(true))
	require.NoError(t, p.ParseFiles(parserTestdataPath("sample_generics.json")))

	// Organize into a scenario
	org := organizer.New(cfg)
	scenario := org.Scenarize(p.Sets())
	require.NotNil(t, scenario)

	// Build the chart page
	builder := New(cfg, scenario)
	page := builder.BuildPage()

	// Render to HTML
	var buf bytes.Buffer
	require.NoError(t, page.Render(&buf))

	html := buf.String()
	require.NotEmpty(t, html)

	// Verify basic HTML structure
	assert.True(t,
		strings.Contains(html, "<html>") || strings.Contains(html, "<!DOCTYPE html>") || strings.Contains(html, "<script"),
		"output doesn't look like HTML",
	)

	// Verify echarts is referenced
	assert.Contains(t, html, "echarts")

	// Write output for manual inspection
	outFile := filepath.Join(t.TempDir(), "smoke_test_output.html")
	require.NoError(t, os.WriteFile(outFile, buf.Bytes(), 0o600))
	t.Logf("HTML output written to: %s (%d bytes)", outFile, buf.Len())
	t.Log(buf.String())
}

// TestSmokeRenderTextFormat tests with plain text benchmark output.
func TestSmokeRenderTextFormat(t *testing.T) {
	cfg := mustLoadConfig(t, smokeConfigText())

	// Parse text benchmark data
	p := parser.New(cfg)
	require.NoError(t, p.ParseFiles(parserTestdataPath("run.txt")))

	org := organizer.New(cfg)
	scenario := org.Scenarize(p.Sets())

	builder := New(cfg, scenario)
	page := builder.BuildPage()

	var buf bytes.Buffer
	require.NoError(t, page.Render(&buf))

	require.NotZero(t, buf.Len())

	t.Logf("text format: rendered %d bytes of HTML", buf.Len())
}

func TestWithTitleAndSubtitle(t *testing.T) {
	c := NewChart(WithTitle("My Title"), WithSubtitle("My Subtitle"))

	assert.Equal(t, "My Title", c.Title)
	assert.Equal(t, "My Subtitle", c.Subtitle)
}

func TestRenderEmptyPage(t *testing.T) {
	page := NewPage("Empty")

	var buf bytes.Buffer
	require.NoError(t, page.Render(&buf))

	assert.NotZero(t, buf.Len())
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

func parserTestdataPath(name string) string {
	return filepath.Join("..", "parser", "testdata", name)
}

func smokeConfig() string {
	return `
name: Smoke Test
render:
  title: Benchmark Comparison
  theme: roma
  chart: barchart
  legend: bottom
  scale: auto

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
    Match: 'Greater'
    NotMatch: 'GreaterOr'
  - id: less
    title: Less
    Match: 'Less'
    NotMatch: 'LessOr'
  - id: positive
    title: Positive
    Match: 'Positive'
  - id: negative
    title: Negative
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
    title: 'Comparisons'
    includes:
      functions: [greater, less, positive, negative]
      versions: [reflect, generics]
      contexts: [int, float64]
      metrics: [nsPerOp, allocsPerOp]
`
}

func smokeConfigText() string {
	return `
name: Text Smoke Test
render:
  title: JSON Library Benchmarks
  theme: roma
  chart: barchart
  legend: bottom
  scale: auto

metrics:
  - id: nsPerOp
    title: Benchmark Timings
    axis: 'ns/op'
  - id: allocsPerOp
    title: Benchmark Allocations
    axis: 'allocs/op'

functions:
  - id: readjson
    title: ReadJSON
    Match: 'ReadJSON'
  - id: writejson
    title: WriteJSON
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
  - id: json-benchmarks
    title: 'JSON Library Performance'
    includes:
      functions: [readjson, writejson]
      versions: [stdlib, easyjson]
      contexts: [small, medium, large]
      metrics: [nsPerOp, allocsPerOp]
`
}
