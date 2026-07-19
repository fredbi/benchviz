# Configuration Reference

`benchviz` is configured with a YAML file (default: `config.yaml`).

This document describes all available configuration fields.

## Top-level fields

| Field         | Type     | Description                                                          |
|---------------|----------|----------------------------------------------------------------------|
| `name`        | string   | Name of the benchmark scenario (used as the HTML page title).        |
| `environment` | string   | Override for the environment label. When empty, extracted from input. |
| `render`      | object   | Chart rendering settings. See [Rendering](#rendering).               |
| `metrics`     | list     | Metric definitions. See [Metrics](#metrics).                         |
| `functions`   | list     | Function definitions. See [Functions](#functions).                    |
| `contexts`    | list     | Context definitions. See [Contexts](#contexts).                      |
| `versions`    | list     | Version definitions. See [Versions](#versions).                      |
| `categories`  | list     | Category definitions. See [Categories](#categories).                 |
| `files`       | list     | File-based matching rules. See [Files](#files).                      |

## Rendering

The `render` section controls how charts look.

```yaml
render:
  title: 'Benchmark'
  theme: roma
  chart: barchart
  layout:
    horizontal: 2
    vertical: 0
  legend: bottom
  scale: auto
  orientation: horizontal
  labelFontSize: 12
  screenshot:
    width: 1920
    height: 1080
    sleep: 1s
```

| Field         | Type   | Default      | Description                                                         |
|---------------|--------|--------------|---------------------------------------------------------------------|
| `title`       | string |              | Chart title prefix.                                                 |
| `theme`       | string | `roma`       | ECharts color theme. See [Themes](#themes).                         |
| `chart`       | string | `barchart`   | Chart type (currently only `barchart` is supported).                 |
| `legend`      | string | `bottom`     | Legend position: `none`, `bottom`, `top`, `left`, `right`.           |
| `scale`       | string | `auto`       | Y-axis scaling: `auto` or `log`.                                    |
| `dualscale`   | bool   | `false`      | Enable dual Y-axis for categories with two metrics.                 |
| `orientation` | string | `vertical`   | Bar direction: `vertical` or `horizontal`.                          |
| `labelFontSize` | int  | `12`       | Font size (px) of the workload axis tick labels. Lower it when long workload names overflow (notably on horizontal bar charts). `0` uses the ECharts default. |

### Layout

The `layout` sub-section controls how multiple charts are arranged on the page.

| Field        | Type | Default | Description                                                       |
|--------------|------|---------|-------------------------------------------------------------------|
| `horizontal` | int  | `0`     | Number of charts per row. `0` or `1` uses the default fixed width.|
| `vertical`   | int  | `0`     | Number of chart rows. `0` uses default height.                    |

Charts are sized in pixels so the flex-wrap page lays them out reliably.
`horizontal: 2` yields the default 900×500 canvas (two charts per row on a
wide viewport); `horizontal: 3` makes each chart proportionally narrower, and
so on. `vertical` divides the nominal page height the same way.

### Screenshot

The `screenshot` sub-section configures the headless Chrome PNG renderer.

| Field    | Type   | Default | Description                                         |
|----------|--------|---------|-----------------------------------------------------|
| `width`  | int    | `1920`  | Viewport width in pixels.                           |
| `height` | int    | `1080`  | Viewport height in pixels.                          |
| `sleep`  | string | `1s`    | Duration to wait for JS rendering (Go duration).    |

### Themes

Available built-in themes from go-echarts:

`roma`, `vintage`, `dark`, `westeros`, `essos`, `wonderland`, `walden`,
`chalk`, `infographic`, `macarons`, `purple-passions`, `shine`.

## Metrics

Each metric selects which benchmark measurement to plot.

```yaml
metrics:
  - id: nsPerOp
    title: Benchmark Timings
    axis: 'ns/op'
```

| Field   | Type   | Description                                    |
|---------|--------|------------------------------------------------|
| `id`    | string | Metric identifier. Must be one of the values below. |
| `title` | string | Display title. Auto-generated from ID if empty.|
| `axis`  | string | Y-axis label text (e.g. `ns/op`).              |

Valid metric IDs:

| ID            | Go benchmark field   |
|---------------|----------------------|
| `nsPerOp`     | `NsPerOp`            |
| `allocsPerOp` | `AllocsPerOp`        |
| `bytesPerOp`  | `AllocedBytesPerOp`  |
| `MBytesPerS`  | `MBPerS`             |

## Functions

Functions identify *what* is being benchmarked by matching on the benchmark name.

```yaml
functions:
  - id: greater
    title: Greater
    match: 'Greater'
    notmatch: 'GreaterOr'
```

| Field      | Type   | Description                                                   |
|------------|--------|---------------------------------------------------------------|
| `id`       | string | Unique identifier (used in category references).              |
| `title`    | string | Display title. Auto-generated from ID if empty.               |
| `match`    | string | Go regexp that must match the benchmark name.                 |
| `notmatch` | string | Go regexp that excludes matching names. Optional.             |

The first function whose `match` regexp hits (and `notmatch` does not) wins.
Benchmarks that don't match any function are skipped.

## Contexts

Contexts identify the *conditions* under which a benchmark runs (e.g. input type, workload size).
They share the same structure as functions.

```yaml
contexts:
  - id: int
    title: int
    match: 'int'
```

Each context becomes one data point in a bar chart series.

## Versions

Versions identify *which implementation* is being compared (e.g. `reflect` vs `generics`).
They share the same structure as functions.

```yaml
versions:
  - id: reflect
    title: reflect
    match: 'reflect'
```

Each version becomes a separate bar series in the chart, shown side by side.

## Categories

A category bundles a subset of functions, versions, contexts, and metrics into a single chart.
Each category produces one chart on the output page (one chart per included metric).

```yaml
categories:
  - id: comparisons
    title: '{metric} (comparisons)'
    includes:
      functions:
        - greater
        - less
      versions:
        - reflect
        - generics
      contexts:
        - int
        - float64
      metrics:
        - nsPerOp
        - allocsPerOp
```

| Field      | Type   | Description                                                                |
|------------|--------|----------------------------------------------------------------------------|
| `id`       | string | Unique identifier.                                                         |
| `title`    | string | Chart title. `{metric}` is replaced with the metric title at render time.  |
| `includes` | object | References to functions, versions, contexts, and metrics by their IDs.     |

The `includes` sub-fields:

| Field       | Type     | Description                                             |
|-------------|----------|---------------------------------------------------------|
| `functions` | []string | Function IDs to include. If empty, all functions apply. |
| `versions`  | []string | Version IDs to include. If empty, all versions apply.   |
| `contexts`  | []string | Context IDs to include. If empty, all contexts apply.   |
| `metrics`   | []string | Metric IDs to include. At least one is required.        |

## Files

File-based rules assign versions or contexts based on the input filename
rather than the benchmark name. This is useful when comparing benchmark runs
from different files (e.g. different machines or git commits).

```yaml
files:
  - id: machine-a
    matchfile: 'machine-a'
    versions:
      - id: v1
        match: 'v1'
    contexts:
      - id: linux
        match: 'linux'
```

| Field       | Type   | Description                                          |
|-------------|--------|------------------------------------------------------|
| `id`        | string | Unique identifier for the file rule.                 |
| `matchfile` | string | Go regexp matched against the input filename.        |
| `versions`  | list   | Version definitions scoped to this file rule.        |
| `contexts`  | list   | Context definitions scoped to this file rule.        |

File-based matching is tried as a fallback when name-based matching for
versions or contexts produces no result.

## Minimal example

```yaml
name: my benchmarks
render:
  theme: roma
  legend: bottom
  orientation: vertical

metrics:
  - id: nsPerOp
    title: Timing
    axis: 'ns/op'

functions:
  - id: sort
    match: 'BenchmarkSort'

contexts:
  - id: small
    match: 'small'
  - id: large
    match: 'large'

categories:
  - id: all
    title: 'Sort Performance'
    includes:
      metrics:
        - nsPerOp
```
