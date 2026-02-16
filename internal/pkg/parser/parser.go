package parser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/fredbi/benchviz/internal/pkg/config"
	"golang.org/x/tools/benchmark/parse"
)

// Set wraps [parse.Set] to include file and benchmark environment information.
type Set struct {
	parse.Set

	File        string
	Environment string
}

// ParsingReport allows to inspect the contents of a parsed benchmark.
type ParsingReport struct {
	NumberOfSets  int           `json:"sets"`
	AnalyzedFiles []string      `json:"analyzed_files"`
	Functions     []string      `json:"benchmark_functions"`
	Metrics       []MinMaxRange `json:"benchmark_metrics"`
	Signatures    []Signature   `json:"benchmark_signatures"`
}

type Signature struct {
	Name             string        `json:"benchmark_name"`
	AvailableMetrics []MinMaxRange `json:"available_metrics"`
	Environment      string        `json:"environment"`
}

type MinMaxRange struct {
	Metric  config.MetricName `json:"metric"`
	Count   int               `json:"measurements_count"`
	Min     float64           `json:"min_value"`
	Max     float64           `json:"max_value"`
	Origins []string          `json:"origin_files"`
}

// Report produces a [ParsingReport], which allows for closer inspection of the content
// of parsed input.
func (p *BenchmarkParser) Report() ParsingReport {
	const sensibleAllocs = 10
	r := ParsingReport{
		Signatures: make([]Signature, 0, sensibleAllocs),
	}
	seenFiles := make(map[string]struct{})
	seenSignatures := make(map[string]struct{})
	seenMetrics := make(map[config.MetricName]int)

	for _, set := range p.sets {
		r.NumberOfSets++
		_, seenFile := seenFiles[set.File]
		if !seenFile {
			seenFiles[set.File] = struct{}{}
			r.AnalyzedFiles = append(r.AnalyzedFiles, set.File)
		}

		for _, benchmarks := range set.Set {
			for _, bench := range benchmarks {
				_, seenSignature := seenSignatures[bench.Name]
				if !seenSignature {
					seenSignatures[bench.Name] = struct{}{}
					r.Functions = append(r.Functions, bench.Name)
				}

				r.Signatures = append(r.Signatures, Signature{
					Name:             bench.Name,
					Environment:      set.Environment,
					AvailableMetrics: extractMetrics(bench, set.File),
				})
			}
		}
	}

	for _, s := range r.Signatures {
		for _, m := range s.AvailableMetrics {
			idx, seenMetric := seenMetrics[m.Metric]
			if !seenMetric {
				seenMetrics[m.Metric] = len(r.Metrics)
				r.Metrics = append(r.Metrics, m)

				continue
			}

			previous := r.Metrics[idx]
			if m.Min < previous.Min {
				previous.Min = m.Min
			}
			if m.Max > previous.Max {
				previous.Max = m.Max
			}
			if len(m.Origins) > 0 && !slices.Contains(previous.Origins, m.Origins[0]) {
				previous.Origins = append(previous.Origins, m.Origins[0])
			}
			previous.Count++
			r.Metrics[idx] = previous
		}
	}

	sort.Strings(r.Functions)

	return r
}

func extractMetrics(bench *parse.Benchmark, file string) (metrics []MinMaxRange) {
	if bench.NsPerOp > 0 {
		metrics = append(metrics, MinMaxRange{
			Metric:  config.MetricNsPerOp,
			Min:     bench.NsPerOp,
			Max:     bench.NsPerOp,
			Origins: []string{file},
			Count:   1,
		})
	}
	if bench.AllocsPerOp > 0 {
		metrics = append(metrics, MinMaxRange{
			Metric:  config.MetricAllocsPerOp,
			Min:     float64(bench.AllocsPerOp),
			Max:     float64(bench.AllocsPerOp),
			Origins: []string{file},
			Count:   1,
		})
	}
	if bench.AllocedBytesPerOp > 0 {
		metrics = append(metrics, MinMaxRange{
			Metric:  config.MetricBytesPerOp,
			Min:     float64(bench.AllocedBytesPerOp),
			Max:     float64(bench.AllocedBytesPerOp),
			Origins: []string{file},
			Count:   1,
		})
	}
	if bench.MBPerS > 0 {
		metrics = append(metrics, MinMaxRange{
			Metric:  config.MetricMBPerS,
			Min:     bench.MBPerS,
			Max:     bench.MBPerS,
			Origins: []string{file},
			Count:   1,
		})
	}

	return metrics
}

type BenchmarkParser struct {
	options

	config *config.Config
	sets   []Set
	l      *slog.Logger
}

// New [BenchmarkParser] ready to parse benchmark files.
func New(cfg *config.Config, opts ...Option) *BenchmarkParser {
	return &BenchmarkParser{
		options: optionsWithDefaults(opts),
		config:  cfg,
		l:       slog.Default().With(slog.String("module", "parser")),
	}
}

func (p *BenchmarkParser) ParseFiles(files ...string) error {
	for _, file := range files {
		var (
			reader io.ReadCloser
			err    error
		)

		if file == "-" {
			reader = os.Stdin
		} else {
			reader, err = os.Open(file)
			if err != nil {
				return fmt.Errorf("input file %q: %w", file, err)
			}
		}

		set, err := p.ParseInput(reader)
		if err != nil {
			if file != "-" {
				_ = reader.Close()
			}

			return err
		}

		set.File = file
		p.sets = append(p.sets, set)

		if file != "-" {
			_ = reader.Close()
		}
	}

	p.l.Info("benchmark input parsed", slog.Int("parsed_files", len(files)))

	return nil
}

func (p *BenchmarkParser) ParseInput(r io.Reader) (Set, error) {
	if p.isJSON {
		return p.parseJSON(r)
	}

	return p.parseText(r)
}

func (p *BenchmarkParser) Sets() []Set {
	return p.sets
}

func (p *BenchmarkParser) parseText(r io.Reader) (Set, error) {
	// Read all input to extract environment info
	content, err := io.ReadAll(r) // TODO: replace with io.TeeReader
	if err != nil {
		return Set{}, fmt.Errorf("reading input: %w", err)
	}

	// Extract environment info
	environment := extractEnvironment(string(content))

	// Parse benchmarks
	set, err := parse.ParseSet(strings.NewReader(string(content)))
	if err != nil {
		return Set{}, err
	}

	s := Set{
		Set:         set,
		Environment: environment,
	}

	return s, nil
}

// parseJSON parses JSON output from `go test -json -bench`.
// It extracts the Output fields from "output" events and feeds them
// to the standard benchmark parser.
func (p *BenchmarkParser) parseJSON(r io.Reader) (Set, error) {
	// Read JSON events line by line and extract Output fields
	var textOutput strings.Builder
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event testEvent
		if err := json.Unmarshal(line, &event); err != nil { //nolint:musttag // JSON produced uses titleized keys expected by std json/encoding
			// Skip lines that aren't valid JSON (shouldn't happen with -json flag)
			continue
		}

		// Only collect output from "output" action events
		if event.Action == "output" && event.Output != "" {
			textOutput.WriteString(event.Output)
		}
	}

	if err := scanner.Err(); err != nil {
		return Set{}, fmt.Errorf("scanning input: %w", err)
	}

	// Extract environment info
	outputText := textOutput.String()
	environment := extractEnvironment(outputText)

	// Now parse the collected text output using the standard parser
	set, err := parse.ParseSet(strings.NewReader(outputText))
	if err != nil {
		return Set{}, fmt.Errorf("parsing benchmark output: %w", err)
	}

	s := Set{
		Set:         set,
		Environment: environment,
	}

	return s, nil
}

// extractEnvironment extracts environment information from benchmark output.
// It looks for goos, goarch, and cpu lines and combines them.
func extractEnvironment(text string) string {
	var parts []string
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(line, "goos: "):
			parts = append(parts, strings.TrimPrefix(line, "goos: "))
		case strings.HasPrefix(line, "goarch: "):
			parts = append(parts, strings.TrimPrefix(line, "goarch: "))
		case strings.HasPrefix(line, "cpu: "):
			cpu := strings.TrimPrefix(line, "cpu: ")
			cpu = strings.TrimSpace(cpu)
			parts = append(parts, "cpu: "+cpu)
		}
	}

	if len(parts) == 0 {
		return "unknown environment"
	}

	return strings.Join(parts, " ")
}

// testEvent represents a single JSON event from `go test -json` output.
// See: https://pkg.go.dev/cmd/test2json
type testEvent struct {
	Time    string
	Action  string
	Package string
	Test    string
	Output  string
	Elapsed float64
}
