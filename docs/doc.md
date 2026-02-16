# benchviz internals

This document describes how `benchviz` operates, from input to output.

## Overview

`benchviz` is a CLI tool that turns Go benchmark output into visual bar charts.
It reads benchmark results (text or JSON), classifies each result using regex-based rules
from a YAML configuration, then renders one or more bar charts on an HTML page.
Optionally, it can take a PNG screenshot of that page using headless Chrome.

The pipeline has four stages:

```
  input files ──► parser ──► organizer ──► chart builder ──► HTML / PNG
```

## 1. Configuration (`internal/pkg/config`)

Everything is driven by a YAML configuration file. The config defines five
families of objects that control how raw benchmark data is sliced and displayed:

### Metrics

A metric selects which measurement to plot. Four metric names are recognized,
matching the fields produced by the Go benchmark harness:

| ID            | Benchmark field    |
|---------------|--------------------|
| `nsPerOp`     | `NsPerOp`          |
| `allocsPerOp` | `AllocsPerOp`      |
| `bytesPerOp`  | `AllocedBytesPerOp`|
| `MBytesPerS`  | `MBPerS`           |

A config must declare at least the metrics it cares about; the organizer
only emits data points for metrics that are present in the config.

### Functions, Versions, Contexts

These three types share the same structure (`Object`): an `id`, an optional
`title`, a `Match` regex and an optional `NotMatch` regex. They classify each
benchmark name into a triple `(function, version, context)`:

- **Function** identifies *what* is being benchmarked (e.g. `Greater`, `ReadJSON`).
- **Version** identifies *which implementation* (e.g. `reflect` vs `generics`).
- **Context** identifies *under which conditions* (e.g. `int`, `float64`, `small`, `large`).

Matching works by scanning the full benchmark name (e.g.
`BenchmarkPositive/reflect/int-16`) against each object's compiled `Match` regex,
excluding names that hit `NotMatch`. The first match wins.

When no version or context is found by name matching, the config supports a
secondary `files` section where version/context can be inferred from the input
file name instead.

### Categories

A category bundles a subset of `(functions x versions x contexts x metrics)`
into a single chart. Each category becomes one bar chart on the output page.
Up to two metrics per category are allowed (for dual-scale rendering).

If a category omits `functions`, `contexts` or `versions` in its `includes`,
all defined objects of that type are injected as defaults.

### Validation

On load, the config:
1. Parses YAML via `go.yaml.in/yaml/v3` then decodes into structs via `mapstructure`.
2. Builds index maps for O(1) lookup of functions, versions, contexts and metrics.
3. Validates uniqueness of IDs, checks metric names against the known set,
   verifies that category references point to existing objects.
4. Compiles all `Match`/`NotMatch` regexps.
5. Auto-titles any object that has an empty `Title` by titleizing its ID
   (e.g. `"elements-match"` becomes `"Elements Match"`).

A default config is embedded via `go:embed` and can serve as a template.

## 2. Parsing (`internal/pkg/parser`)

The parser reads benchmark data from one or more files (or stdin with `-`).

Two input formats are supported:

- **Text**: standard `go test -bench` output. Parsed directly by
  `golang.org/x/tools/benchmark/parse.ParseSet`.
- **JSON**: `go test -json -bench` output. Each line is a JSON event
  (`test2json` format). The parser extracts `Output` fields from `"output"`
  action events, reassembles them into text, then feeds that text to
  `parse.ParseSet`.

The parser also extracts environment metadata (`goos`, `goarch`, `cpu`)
from the preamble lines of the benchmark output.

The result is a slice of `parser.Set`, each wrapping a `parse.Set` (a
`map[string][]*parse.Benchmark`) together with the source file name and
extracted environment string.

Note: `parse.ParseSet` retains the GOMAXPROCS suffix in benchmark names
(e.g. `BenchmarkFoo-16`), so all downstream regex matching must account for it.

## 3. Organizing (`internal/pkg/organizer`)

The organizer transforms raw parsed data into a structured `model.Scenario`
ready for chart rendering.

### Step 1: classify benchmarks

For each benchmark in each parsed set, `parseBenchmarkName` applies the
config's regex rules to extract a `(function, version, context)` triple.
Benchmarks that don't match any function are discarded with a warning.

For each matched benchmark, the organizer emits one `ParsedBenchmark` per
configured metric, extracting the corresponding value from the
`parse.Benchmark` struct.

### Step 2: populate categories

For each category in the config, the organizer iterates over
`metrics x versions` and calls `SeriesFor` to extract the data series.

`SeriesFor` iterates `functions x contexts` (in config order) and collects
matching data points. Each function becomes one `MetricSeries` (one bar
group), with one `MetricPoint` per context. The series title is the version
name (used in the chart legend).

The result is a `model.Scenario` containing a list of `model.Category`,
each with its `CategoryData` slices.

## 4. Chart rendering (`internal/pkg/chart`)

### Building

`chart.Builder.BuildPage()` creates a `Page` (an HTML page with multiple charts).
For each `model.Category` that has data, it builds a `Chart` using the
functional options pattern (`WithTitle`, `WithSubtitle`, `WithYAxisLabel`,
`WithLegend`, `WithTheme`).

Each chart accumulates `Series` by iterating over the category's data.
A `Series` maps to an ECharts bar data series.

### Rendering to HTML

`Page.Render(w)` uses `go-echarts/components.Page` to compose all charts
onto a single HTML page with flex layout. Each `Chart.Build()` produces a
`charts.Bar` with configured:
- Title and subtitle (subtitle typically shows the benchmark environment).
- X-axis with category labels, rotated 30 degrees.
- Y-axis with the metric label and auto-scaling.
- Legend at the bottom-right.
- A "save as image" toolbox button.
- The `roma` theme (or another configured theme).

The HTML output is self-contained: it includes the ECharts JS library inline.

## 5. Image rendering (`internal/pkg/image`)

When a PNG output is requested, the image renderer:
1. Reads the generated HTML.
2. Launches a headless Chrome instance via `chromedp`.
3. Navigates to the HTML content using a `data:text/html,` URL.
4. Waits one second for JavaScript rendering to complete.
5. Takes a full-screen PNG screenshot at 1920x1080.
6. Writes the PNG bytes to the output.

## 6. CLI (`internal/cmd`)

The CLI is a thin `flag`-based interface:

```
benchviz [-json] [-config config.yaml] [-output file] [-environment env] [files...]
```

### Flag processing

| Flag | Default | Description |
|------|---------|-------------|
| `-json` | `false` | Parse input as JSON (`go test -json`) |
| `-config`, `-c` | `config.yaml` | YAML configuration file |
| `-output`, `-o` | `-` (stdout) | Output file path |
| `-environment`, `-e` | `-` | Environment label override |

### Output resolution

The `-output` flag determines what gets produced:

- **`-` (stdout)**: HTML is written to stdout, no PNG.
- **`file.html`**: HTML is written to that file. If the config also specifies
  a PNG file, a PNG is inferred as `file.png`.
- **`file.png`**: the HTML extension is inferred as `file.html`. If there's a
  pre-existing PNG config, it's overridden to match.
- When the config has a `PngFile` but no `HTMLFile`, a temporary HTML file is
  created (cleaned up after PNG rendering).

### Execution pipeline

`Execute` orchestrates the full pipeline:

1. Load and validate the YAML config.
2. Apply CLI flag overrides (`setConfig`).
3. Parse all input files via the parser.
4. Organize into a scenario via the organizer.
5. Build the chart page via the chart builder.
6. Render HTML to the output file (or stdout).
7. If a PNG is requested, re-read the HTML and render it to PNG via headless Chrome.

## Data flow diagram

```
                          config.yaml
                              │
                              ▼
                     ┌────────────────┐
                     │  config.Load   │
                     │  validate +    │
                     │  compile regex │
                     └───────┬────────┘
                             │
                        *config.Config
                             │
          ┌──────────────────┼──────────────────┐
          ▼                  ▼                   ▼
   ┌─────────────┐   ┌─────────────┐    ┌──────────────┐
   │   parser     │   │  organizer  │    │ chart builder│
   │             │   │             │    │              │
   │ text/JSON   │──►│ classify +  │───►│ build page + │
   │ → parse.Set │   │ scenarize   │    │ bar charts   │
   └─────────────┘   └─────────────┘    └──────┬───────┘
                                               │
                                          chart.Page
                                               │
                              ┌────────────────┼────────────────┐
                              ▼                                 ▼
                      ┌──────────────┐                  ┌──────────────┐
                      │  HTML output │                  │  PNG output  │
                      │  (go-echarts)│                  │  (chromedp)  │
                      └──────────────┘                  └──────────────┘
```

## Key dependencies

| Library | Purpose |
|---------|---------|
| `golang.org/x/tools/benchmark/parse` | Parse standard Go benchmark text output |
| `go.yaml.in/yaml/v3` | YAML config parsing |
| `github.com/go-viper/mapstructure/v2` | Decode YAML maps into typed structs |
| `github.com/go-echarts/go-echarts/v2` | Generate ECharts-based HTML bar charts |
| `github.com/chromedp/chromedp` | Headless Chrome for HTML-to-PNG screenshots |
| `golang.org/x/text/cases` | Title-case conversion for auto-generated titles |
