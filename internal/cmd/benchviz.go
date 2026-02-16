// Package cmd owns the implementation details of the CLI command.
package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path"
	"strings"
	"time"

	"github.com/fredbi/benchviz/internal/pkg/chart"
	"github.com/fredbi/benchviz/internal/pkg/config"
	"github.com/fredbi/benchviz/internal/pkg/image"
	"github.com/fredbi/benchviz/internal/pkg/organizer"
	"github.com/fredbi/benchviz/internal/pkg/parser"
)

// Command holds command line flags and executes the benchviz command.
//
// It knows how to load a configuration file in a [config.Config] and manage CLI flag configuration overrides.
//
// The main purpose of this package is to deal with io's: opening and closing files.
//
// All other invoked functionalities deal with streams, except the benchmark parser which may collect several files
// directly.
type Command struct {
	Config      string
	OutputFile  string
	IsJSON      bool
	Environment string
	Report      bool
	Png         bool
	L           *slog.Logger
}

// NewCommand builds a CLI command with registered flags and an injected logger.
func NewCommand() *Command {
	// inject a structured logger
	cli := &Command{
		L: slog.Default().With(slog.String("module", "main")),
	}

	cli.registerFlags()

	return cli
}

// Parse command line flags and arguments.
func (*Command) Parse() error {
	return flag.CommandLine.Parse(os.Args[1:])
}

// Fatalf logs an error message then exits. The output is spewed on both stderr and the structured logger output.
func (c *Command) Fatalf(err error) {
	c.L.Error(err.Error())
	log.Fatalf("%v", err)
}

// Execute the CLI with flags and extra arguments.
//
// If no argument is passed, command line arguments (i.e. [os.Args]) are used.
func (c *Command) Execute(args ...string) error {
	if args == nil { // passing explicit args allows for testing Execute without altering [os.Args]
		args = c.args()
	}
	if len(args) == 0 { // no file is provided: assume stdin
		args = append(args, "-")
	}

	cfg, cleanup, err := c.prepareConfig()
	if err != nil {
		return err
	}
	defer cleanup()

	if c.Report {
		// just want to report about the content of the benchmark files
		return c.report(cfg, args)
	}

	// 1. parse benchmark parses input benchmark files and build a chart page
	htmlRenderer, err := buildPage(cfg, args)
	if err != nil {
		return err
	}

	// 2. render the page as HTML, possibly to stdout, possibly to temp file
	htmlWriter, htmlCloser, err := getWriter(cfg.Outputs.HTMLFile, "HTML")
	if err != nil {
		return err
	}

	if err := htmlRenderer.Render(htmlWriter); err != nil {
		htmlCloser()
		return fmt.Errorf("rendering page: %w", err)
	}

	htmlCloser()

	if cfg.Outputs.PngFile == "" {
		// html only: we're done
		return nil
	}

	// 3. convert the HTML page to a PNG image, possibly to stdout
	htmlReader, htmlCloser, err := getReader(cfg.Outputs.HTMLFile, "HTML")
	if err != nil {
		return err
	}

	pngWriter, pngCloser, err := getWriter(cfg.Outputs.PngFile, "PNG")
	if err != nil {
		htmlCloser()
		return err
	}

	defer pngCloser()

	r := image.New()

	if err = r.Render(pngWriter, htmlReader); err != nil {
		return fmt.Errorf("rendering image: %w", err)
	}

	return nil
}

func (*Command) args() []string {
	return flag.CommandLine.Args()
}

func (c *Command) registerFlags() {
	defaults := Command{
		Config:      "benchviz.yaml",
		OutputFile:  "-",
		Png:         false,
		IsJSON:      false,
		Environment: "",
		Report:      false,
	}

	flag.BoolVar(&c.IsJSON, "json", defaults.IsJSON, "read input from JSON")
	flag.StringVar(&c.Config, "config", defaults.Config, "config file")
	flag.StringVar(&c.Config, "c", defaults.Config, "config file (shorthand)")
	flag.StringVar(&c.OutputFile, "output", defaults.OutputFile, "file output or - for standard output")
	flag.StringVar(&c.OutputFile, "o", defaults.OutputFile, "file output or - for standard output (shorthand)")
	flag.StringVar(&c.Environment, "environment", defaults.Environment, "environment string")
	flag.StringVar(&c.Environment, "e", defaults.Environment, "environment string (shorthand)")
	flag.BoolVar(&c.Report, "r", defaults.Report, "report benchmark contents only, no rendering (shorthand)")
	flag.BoolVar(&c.Report, "report", defaults.Report, "report benchmark contents only")
	flag.BoolVar(&c.Png, "png", defaults.Png, "enable PNG screenshot output")
}

func (c *Command) prepareConfig() (cfg *config.Config, cleanup func(), err error) {
	cfg, err = config.Load(c.Config)
	if err != nil {
		return nil, nil, fmt.Errorf("loading config: %w", err)
	}

	if err = c.setConfig(cfg); err != nil {
		return nil, nil, fmt.Errorf("preparing config: %w", err)
	}

	if cfg.Outputs.IsTemp && !c.Report {
		cleanup = func() {
			_ = os.Remove(cfg.Outputs.HTMLFile)
		}

		return cfg, cleanup, err
	}

	return cfg, func() {}, err
}

// apply CLI flags overrides to YAML config.
func (c *Command) setConfig(cfg *config.Config) error {
	cfg.IsJSON = c.IsJSON

	if c.Environment != "" {
		cfg.Environment = c.Environment
	}

	if c.OutputFile != "" && c.OutputFile != "-" {
		// an outfile is defined: infer the PNG file from the HTML file provided
		cfg.Outputs.HTMLFile = inferHTMLFile(c.OutputFile)
		if cfg.Outputs.PngFile == "" && c.Png {
			cfg.Outputs.PngFile = inferImageFile(cfg.Outputs.HTMLFile)
		}
	}

	if c.Report {
		return nil
	}

	switch {
	case cfg.Outputs.HTMLFile == "" && cfg.Outputs.PngFile == "":
		c.L.Info("output sent to standard output as HTML, no PNG image rendered")
		if c.Png {
			c.L.Info("set an output file to render a PNG image")
		}
		cfg.Outputs.HTMLFile = "-"
	case cfg.Outputs.HTMLFile == "" && cfg.Outputs.PngFile != "":
		c.L.Info("HTML generated as a temporary file to produce PNG")
		tmp, err := os.CreateTemp("", "benchviz.*.html")
		if err != nil {
			return err
		}
		cfg.Outputs.HTMLFile = tmp.Name()
		cfg.Outputs.IsTemp = true
		_ = tmp.Close()
	}

	return nil
}

// report produces a report that explores the input benchmarks.
func (c *Command) report(cfg *config.Config, args []string) error {
	p := parser.New(cfg, parser.WithParseJSON(cfg.IsJSON))
	t0 := time.Now()
	if err := p.ParseFiles(args...); err != nil {
		return fmt.Errorf("parsing files: %w", err)
	}
	c.L.Info("parsed input benchmarks", slog.Duration("duration", time.Now().Sub(t0)))

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")

	return enc.Encode(p.Report())
}

func getReader(file, kind string) (rdr *os.File, cleanup func(), err error) {
	rdr, err = os.Open(file)
	if err != nil {
		return nil, nil, fmt.Errorf("opening %s file: %q: %w", kind, file, err)
	}

	cleanup = func() {
		_ = rdr.Close()
	}

	return rdr, cleanup, nil
}

func getWriter(file, kind string) (wrt *os.File, cleanup func(), err error) {
	wrt, err = os.Create(file)
	if err != nil {
		return nil, nil, fmt.Errorf("opening %s file for writing: %q: %w", kind, file, err)
	}

	cleanup = func() {
		_ = wrt.Close()
	}

	return wrt, cleanup, nil
}

func buildPage(cfg *config.Config, args []string) (*chart.Page, error) {
	// 1. parse input benchmarks passed as CLI args
	p := parser.New(cfg, parser.WithParseJSON(cfg.IsJSON))
	if err := p.ParseFiles(args...); err != nil {
		return nil, fmt.Errorf("parsing files: %w", err)
	}

	// 2. re-organize the data series according to the configuration
	o := organizer.New(cfg)
	scenario := o.Scenarize(p.Sets())

	// 3. build a page with this visualization scenario
	builder := chart.New(cfg, scenario)
	page := builder.BuildPage()

	return page, nil
}

func inferHTMLFile(base string) string {
	ext := path.Ext(base)
	image, _ := strings.CutSuffix(base, ext)

	return image + ".html"
}

func inferImageFile(base string) string {
	ext := path.Ext(base)
	image, _ := strings.CutSuffix(base, ext)

	return image + ".png"
}
