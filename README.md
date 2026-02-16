# benchviz

A rendering tool for go benchmarks.

## Install

```sh
go install github.com/fredbi/benchviz@latest
```

## Features

`benchviz` happily slurps output from your go benchmarks (text or JSON),
and renders them as nice bar charts on a single page.

The ouput format may be a HTML page or a PNG screenshot of that page.

## Requirements

`go1.25`

## Concepts

So this is a chart drawing utility, with some pre-baked logic specifically targeted at
rendering benchmark data.

A YAML configuration allows for different rendering scenarios.

We rearrange raw measurements into series to be rendered as bar charts.

1. Metrics: represent which benchmark measurement to be displayed.
   Usually, a single metric is displayed on a given chart.
   Dual-scale charts displaying 2 metrics on the same chart is supported on option.
2. Functions: represent which measurement series to extract.
   Functions are identified by regular expressions.
3. Categories: represent an individual chart on the page. You may pack several such charts on the same page.
   A category is a bundle of (functions x contexts x versions).
4. Context: criterion to build the points of a single series displayed as a bar chart.
   Examples: you may want to render the performance of a given function under different workloads.
5. Version: criterion to render series side by side.
   Examples: you may want to render side by side 2 different versions of the same function,
   or runs on different environments.

All these items may get a customized title.

## Layout options

* theme
* bar chart layout: horizontal or vertical bars
* axis labels
* legend

## Examples

## Acknowledgements

This tool leverages two fantastics libraries.

1. To build charts:
  ```
	github.com/go-echarts/go-echarts/v2 v2.6.7
  ```
2. To take screenshots of a HTML page

  ```
	github.com/chromedp/chromedp 
  ```
