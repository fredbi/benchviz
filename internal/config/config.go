package config

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"go.yaml.in/yaml/v3"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

//go:embed default_config.yaml
var efs embed.FS

// Config holds the configuration for benchviz.
type Config struct {
	Name        string
	IsJSON      bool `mapstructure:"-"`
	IsStrict    bool `mapstructure:"-"`
	Environment string
	Render      Rendering
	Outputs     Output `mapstructure:"-"`
	Metrics     []Metric
	Functions   []Function
	Contexts    []Context
	Versions    []Version
	Categories  []Category
	Files       []File // Files allows for enrichments based on the input file name

	functionIndex map[string]Function
	contextIndex  map[string]Context
	versionIndex  map[string]Version
	metricIndex   map[MetricName]Metric
	// TODO: provision default context, version for regexp mismatches
}

// GetFunction retrieves a function definition by its ID.
func (c Config) GetFunction(id string) (Function, bool) {
	v, ok := c.functionIndex[id]

	return v, ok
}

// GetContext retrieves a context definition by its ID.
func (c Config) GetContext(id string) (Context, bool) {
	v, ok := c.contextIndex[id]

	return v, ok
}

// GetVersion retrieves a version definition by its ID.
func (c Config) GetVersion(id string) (Version, bool) {
	v, ok := c.versionIndex[id]

	return v, ok
}

// GetMetric retrieves a metric definition by its [MetricName].
func (c Config) GetMetric(id MetricName) (Metric, bool) {
	v, ok := c.metricIndex[id]

	return v, ok
}

// FindFunction returns the ID of the first function whose regexp matches the given benchmark name.
func (c Config) FindFunction(name string) (id string, ok bool) {
	for _, def := range c.Functions {
		if id, ok := def.MatchString(name); ok {
			return id, true
		}
	}

	return "", false
}

// FindVersion returns the ID of the first version whose regexp matches the given benchmark name.
func (c Config) FindVersion(name string) (id string, ok bool) {
	for _, def := range c.Versions {
		if id, ok := def.MatchString(name); ok {
			return id, true
		}
	}

	return "", false
}

// FindVersionFromFile returns the ID of the first version matched by a file-based rule.
func (c Config) FindVersionFromFile(file string) (id string, ok bool) {
	for _, def := range c.Files {
		if _, ok := def.MatchString(file); !ok {
			continue
		}

		for _, version := range def.Versions {
			if id, ok := version.MatchString(file); ok {
				return id, true
			}
		}
	}

	return "", false
}

// FindContext returns the ID of the first context whose regexp matches the given benchmark name.
func (c Config) FindContext(name string) (id string, ok bool) {
	for _, def := range c.Contexts {
		if id, ok := def.MatchString(name); ok {
			return id, true
		}
	}

	return "", false
}

// FindContextFromFile returns the ID of the first context matched by a file-based rule.
func (c Config) FindContextFromFile(file string) (id string, ok bool) {
	for _, def := range c.Files {
		if _, ok := def.MatchString(file); !ok {
			continue
		}

		for _, context := range def.Contexts {
			if id, ok := context.MatchString(file); ok {
				return id, true
			}
		}
	}

	return "", false
}

// EncodeYAML serializes a [Config] to YAML into the provided writer.
//
// Runtime-only fields (IsJSON, IsStrict, Outputs) are excluded from the output.
func (c *Config) EncodeYAML(w io.Writer) error {
	var raw map[string]any

	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Squash: true,
		Deep:   true,
		Result: &raw,
	})
	if err != nil {
		return fmt.Errorf("creating mapstructure decoder: %w", err)
	}

	if err := dec.Decode(c); err != nil {
		return fmt.Errorf("decoding config to map: %w", err)
	}

	return yaml.NewEncoder(w).Encode(raw)
}

// Rendering holds chart rendering settings (theme, layout, legend, scale).
type Rendering struct {
	Title       string
	Theme       string
	Layout      Layout
	Chart       string
	Legend      LegendPosition
	Scale       Scale
	DualScale   bool
	Orientation Orientation
	Screenshot  Screenshot
}

// Orientation controls the chart bar direction.
type Orientation string

// Supported chart orientations.
const (
	OrientationVertical   Orientation = "vertical"
	OrientationHorizontal Orientation = "horizontal"
)

// Screenshot configures the headless Chrome screenshot used for PNG rendering.
type Screenshot struct {
	Height int64
	Width  int64
	Sleep  string
}

// SleepDuration parses the Sleep field as a [time.Duration].
func (s Screenshot) SleepDuration() time.Duration {
	d, err := time.ParseDuration(s.Sleep)
	if d == 0 || err != nil {
		return 0
	}

	return d
}

// File defines a file-matching rule that enriches benchmarks with version or context based on filename.
type File struct {
	ID        string
	MatchFile string
	Contexts  []Context
	Versions  []Version

	match *regexp.Regexp
}

// MatchString reports whether the file name matches the file rule, returning the file rule ID.
func (f File) MatchString(file string) (id string, ok bool) {
	if f.match == nil {
		return "", false
	}

	if ok := f.match.MatchString(file); !ok {
		return "", false
	}

	return f.ID, true
}

// Layout controls how charts are arranged on the page.
type Layout struct {
	Horizontal int
	Vertical   int
}

// Scale controls the Y-axis scaling strategy.
type Scale string

// Supported Y-axis scale modes.
const (
	ScaleAuto Scale = "auto"
	ScaleLog  Scale = "log"
)

// LegendPosition controls where the chart legend is displayed.
type LegendPosition string

// Supported legend positions.
const (
	LegendPositionNone   LegendPosition = "none"
	LegendPositionBottom LegendPosition = "bottom"
	LegendPositionTop    LegendPosition = "top"
	LegendPositionLeft   LegendPosition = "left"
	LegendPositionRight  LegendPosition = "right"
)

// Output holds the resolved output file paths for HTML and PNG rendering.
type Output struct {
	HTMLFile string
	PngFile  string
	IsTemp   bool
}

// Metric defines a benchmark metric with its display title and axis label.
type Metric struct {
	ID    MetricName
	Title string
	Axis  string
}

// Object is the base type for regexp-matched configuration entries (functions, contexts, versions).
type Object struct {
	ID       string
	Title    string
	Match    string
	NotMatch string
	match    *regexp.Regexp
	notMatch *regexp.Regexp
}

// Matchers returns the compiled positive and negative match regexps.
func (o Object) Matchers() (match, notMatch *regexp.Regexp) {
	return o.match, o.notMatch
}

// MatchString reports whether name matches the object's positive regexp and not its negative regexp.
func (o Object) MatchString(name string) (id string, ok bool) {
	var matchOk, notMatchOk bool
	id = o.ID
	matcher, notMatcher := o.Matchers()

	if matcher == nil && notMatcher == nil {
		return "", false
	}

	if matcher != nil {
		matchOk = matcher.MatchString(name)
	}

	if notMatcher != nil {
		notMatchOk = notMatcher.MatchString(name)
	}

	if matchOk && !notMatchOk {
		return id, true
	}

	if matcher == nil && !notMatchOk {
		return id, true
	}

	return "", false
}

// Function identifies a benchmark function by regexp matching on its name.
type Function struct {
	Object `mapstructure:",deep,squash"`
}

// Context identifies a benchmark context (e.g. input size, data type) by regexp matching.
type Context struct {
	Object `mapstructure:",deep,squash"`
}

// Version identifies a benchmark implementation variant (e.g. "reflect", "generics") by regexp matching.
type Version struct {
	Object `mapstructure:",deep,squash"`
}

// Category groups functions, contexts, versions and metrics into a single chart.
type Category struct {
	ID       string
	Title    string
	Includes Includes
}

// Includes lists the IDs of functions, versions, contexts and metrics included in a [Category].
type Includes struct {
	Functions []string
	Versions  []string
	Contexts  []string
	Metrics   []MetricName
}

// Load a configuration file from the local file system.
func Load(file string) (*Config, error) {
	cfg, err := loadDefaults()
	if err != nil {
		return nil, fmt.Errorf("loading default config: %w", err)
	}

	fsys := os.DirFS(filepath.Dir(file))
	pth := filepath.Join(".", filepath.Base(file))

	return load(fsys, pth, cfg)
}

// LoadDefaults loads the default configuration from the embedded default_config.yaml.
func LoadDefaults() (*Config, error) {
	return loadDefaults()
}

// loadDefaults loads the default configuration from embedded FS.
func loadDefaults() (*Config, error) {
	return load(efs, "default_config.yaml", &Config{})
}

func load(fsys fs.FS, file string, cfg *Config) (*Config, error) {
	content, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, err
	}

	var raw any
	err = yaml.Unmarshal(content, &raw)
	if err != nil {
		return nil, err
	}

	err = mapstructure.Decode(raw, cfg)
	if err != nil {
		return nil, err
	}

	// build indices and validate unique IDs
	cfg.functionIndex = make(map[string]Function, len(cfg.Functions))
	cfg.contextIndex = make(map[string]Context, len(cfg.Contexts))
	cfg.versionIndex = make(map[string]Version, len(cfg.Versions))
	cfg.metricIndex = make(map[MetricName]Metric, len(cfg.Metrics))

	if err = cfg.validateFunctions(); err != nil {
		return nil, err
	}

	if err = cfg.validateContexts(); err != nil {
		return nil, err
	}

	if err = cfg.validateVersions(); err != nil {
		return nil, err
	}

	if err = cfg.validateMetrics(); err != nil {
		return nil, err
	}

	if err = cfg.validateCategories(); err != nil {
		return nil, err
	}

	if err = cfg.validateRegexps(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validateFunctions() error {
	for i, v := range c.Functions {
		if v.ID == "" {
			return fmt.Errorf("invalid functions: empty ID found: functions[%d]", i)
		}
		if _, ok := c.functionIndex[v.ID]; ok {
			return fmt.Errorf("invalid functions: duplicate ID key found: %s", v.ID)
		}
		if v.Title == "" {
			v.Title = titleize(v.ID)
		}
		c.functionIndex[v.ID] = v
	}

	return nil
}

func (c *Config) validateContexts() error {
	for i, v := range c.Contexts {
		if v.ID == "" {
			return fmt.Errorf("invalid contexts: empty ID found: contexts[%d]", i)
		}
		if _, ok := c.contextIndex[v.ID]; ok {
			return fmt.Errorf("invalid contexts: duplicate ID key found: %s", v.ID)
		}
		if v.Title == "" {
			v.Title = titleize(v.ID)
		}
		c.contextIndex[v.ID] = v
	}

	return nil
}

func (c *Config) validateVersions() error {
	for i, v := range c.Versions {
		if v.ID == "" {
			return fmt.Errorf("invalid versions: empty ID found: versions[%d]", i)
		}
		if _, ok := c.versionIndex[v.ID]; ok {
			return fmt.Errorf("invalid versions: duplicate ID key found: %s", v.ID)
		}
		if v.Title == "" {
			v.Title = titleize(v.ID)
		}
		c.versionIndex[v.ID] = v
	}

	return nil
}

func (c *Config) validateMetrics() error {
	for i, v := range c.Metrics {
		if v.ID == "" {
			return fmt.Errorf("invalid metrics: empty ID found: metrics[%d]", i)
		}
		if !v.ID.IsValid() {
			return fmt.Errorf("invalid metrics: invalid metric ID: metrics[%d]=%v (should be one of %v)", i, v.ID, AllMetricNames())
		}
		if v.Title == "" {
			v.Title = titleize(v.ID)
		}
		if _, ok := c.metricIndex[v.ID]; ok {
			return fmt.Errorf("invalid metrics: duplicate ID key found: %s", v.ID)
		}

		c.metricIndex[v.ID] = v
	}

	return nil
}

func (c *Config) validateCategories() (err error) {
	for i, v := range c.Categories {
		v, err = c.validateCategory(v, i)
		if err != nil {
			return err
		}

		c.Categories[i] = v
	}

	return nil
}

func (c *Config) validateCategory(v Category, i int) (vv Category, err error) {
	if v.ID == "" {
		return vv, fmt.Errorf("invalid categories: empty ID found: categories[%d]", i)
	}

	if v.Title == "" {
		v.Title = titleize(v.ID)
	}

	includes := v.Includes
	for j, ref := range includes.Functions {
		_, ok := c.functionIndex[ref]
		if !ok {
			return vv, fmt.Errorf("invalid category: function ID not found categories.%s.includes.functions[%d]=%s", v.ID, j, ref)
		}
	}

	if len(includes.Functions) == 0 {
		for _, injected := range c.Functions {
			v.Includes.Functions = append(v.Includes.Functions, injected.ID)
		}
	}

	for j, ref := range includes.Contexts {
		_, ok := c.contextIndex[ref]
		if !ok {
			return vv, fmt.Errorf("invalid category: context ID not found categories.%s.includes.contexts[%d]=%s", v.ID, j, ref)
		}
	}

	if len(includes.Contexts) == 0 {
		for _, injected := range c.Contexts {
			v.Includes.Contexts = append(v.Includes.Contexts, injected.ID)
		}
	}

	for j, ref := range includes.Versions {
		_, ok := c.versionIndex[ref]
		if !ok {
			return vv, fmt.Errorf("invalid category: version ID not found categories.%s.includes.versions[%d]=%s", v.ID, j, ref)
		}
	}

	if len(includes.Versions) == 0 {
		for _, injected := range c.Versions {
			v.Includes.Versions = append(v.Includes.Versions, injected.ID)
		}
	}

	for j, ref := range includes.Metrics {
		_, ok := c.metricIndex[ref]
		if !ok {
			return vv, fmt.Errorf("invalid category: metric ID not found categories.%s.includes.metrics[%d]=%s", v.ID, j, ref)
		}
	}

	if len(includes.Metrics) == 0 {
		return vv, fmt.Errorf("invalid category: at least 1 metric must be included in a category. category.%s.metrics", v.ID)
	}

	return v, nil
}

func (c *Config) validateRegexps() error {
	// parse all regexps
	for i, container := range c.Functions {
		match, notMatch, err := compileRex(container.Object)
		if err != nil {
			return fmt.Errorf("invalid regexp[function %d - %s]: %w", i, container.ID, err)
		}
		container.match = match
		container.notMatch = notMatch
		c.Functions[i] = container
	}

	for i, container := range c.Contexts {
		match, notMatch, err := compileRex(container.Object)
		if err != nil {
			return fmt.Errorf("invalid regexp[context %d - %s]: %w", i, container.ID, err)
		}
		container.match = match
		container.notMatch = notMatch
		c.Contexts[i] = container
	}

	for i, container := range c.Versions {
		match, notMatch, err := compileRex(container.Object)
		if err != nil {
			return fmt.Errorf("invalid regexp[version %d - %s]: %w", i, container.ID, err)
		}
		container.match = match
		container.notMatch = notMatch
		c.Versions[i] = container
	}

	for i, container := range c.Files {
		if container.ID == "" {
			return fmt.Errorf("missing ID for file in files[%d]", i)
		}

		if container.MatchFile == "" {
			continue
		}

		match, err := regexp.Compile(container.MatchFile)
		if err != nil {
			return err
		}

		container.match = match
		for j, def := range container.Contexts {
			_, ok := c.contextIndex[def.ID]
			if !ok {
				return fmt.Errorf("invalid file: context ID not found files[%d].context[%d]=%s", i, j, def.ID)
			}

			match, notMatch, err := compileRex(def.Object)
			if err != nil {
				return fmt.Errorf("invalid regexp[files[%d].contexts[%d] - %s]: %w", i, j, def.ID, err)
			}
			def.match = match
			def.notMatch = notMatch
			container.Contexts[j] = def
		}

		for j, def := range container.Versions {
			_, ok := c.versionIndex[def.ID]
			if !ok {
				return fmt.Errorf("invalid file: version ID not found files[%d].versions[%d]=%s", i, j, def.ID)
			}

			match, notMatch, err := compileRex(def.Object)
			if err != nil {
				return fmt.Errorf("invalid regexp[files[%d].versions[%d] - %s]: %w", i, j, def.ID, err)
			}
			def.match = match
			def.notMatch = notMatch
			container.Versions[j] = def
		}

		c.Files[i] = container
	}

	return nil
}

func compileRex(o Object) (match, notMatch *regexp.Regexp, err error) {
	if o.Match != "" {
		match, err = regexp.Compile(o.Match)
		if err != nil {
			return nil, nil, err
		}
	}
	if o.NotMatch != "" {
		notMatch, err = regexp.Compile(o.NotMatch)
		if err != nil {
			return nil, nil, err
		}
	}

	return match, notMatch, nil
}

type str interface {
	~string
}

func titleize[T str](in T) string {
	caser := cases.Title(language.English, cases.NoLower) // the case is stateful: cannot declare it globally

	return caser.String(strings.Map(func(r rune) rune {
		switch r {
		case '_', '-':
			return ' '
		default:
			return r
		}
	}, string(in),
	))
}

// GenerateInput holds the data needed by [Generate] to build a configuration
// from parsed benchmark results.
//
// This avoids importing the parser package (which imports [config]).
type GenerateInput struct {
	Functions []string
	Metrics   []MetricName
}

// Generate builds a [Config] from parsed benchmark data.
//
// It creates one function entry per unique benchmark name, includes all detected metrics,
// and bundles everything into a single "all" category.
func Generate(input GenerateInput) *Config {
	defaults, err := loadDefaults()
	if err != nil {
		// embedded config must always parse
		panic(fmt.Sprintf("loading embedded defaults: %v", err))
	}

	cfg := &Config{
		Name:   "Generated Config",
		Render: defaults.Render,
	}

	// build default metric info map from defaults
	defaultMetrics := make(map[MetricName]Metric, len(defaults.Metrics))
	for _, m := range defaults.Metrics {
		defaultMetrics[m.ID] = m
	}

	// metrics
	for _, name := range input.Metrics {
		if dm, ok := defaultMetrics[name]; ok {
			cfg.Metrics = append(cfg.Metrics, dm)
		} else {
			cfg.Metrics = append(cfg.Metrics, Metric{
				ID:    name,
				Title: titleize(name),
			})
		}
	}

	// functions
	seen := make(map[string]struct{})
	for _, name := range input.Functions {
		id := benchNameToID(name)
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}

		cfg.Functions = append(cfg.Functions, Function{
			Object: Object{
				ID:    id,
				Title: titleize(id),
				Match: regexp.QuoteMeta(name),
			},
		})
	}

	// single category bundling everything
	funcIDs := make([]string, 0, len(cfg.Functions))
	for _, f := range cfg.Functions {
		funcIDs = append(funcIDs, f.ID)
	}

	metricIDs := make([]MetricName, 0, len(cfg.Metrics))
	for _, m := range cfg.Metrics {
		metricIDs = append(metricIDs, m.ID)
	}

	cfg.Categories = []Category{
		{
			ID:    "all",
			Title: "All Benchmarks ({metric})",
			Includes: Includes{
				Functions: funcIDs,
				Metrics:   metricIDs,
			},
		},
	}

	return cfg
}

// benchNameToID converts a benchmark function name to a kebab-case ID.
//
// It strips the "Benchmark" prefix and the GOMAXPROCS suffix (e.g. "-16").
func benchNameToID(name string) string {
	// strip "Benchmark" prefix
	id := strings.TrimPrefix(name, "Benchmark")
	// strip leading underscore (e.g. Benchmark_isEmpty -> isEmpty)
	id = strings.TrimPrefix(id, "_")

	// strip GOMAXPROCS suffix like "-16"
	if idx := strings.LastIndex(id, "-"); idx > 0 {
		suffix := id[idx+1:]
		allDigits := true
		for _, r := range suffix {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits && len(suffix) > 0 {
			id = id[:idx]
		}
	}

	// convert slashes and underscores to hyphens, lowercase
	id = strings.Map(func(r rune) rune {
		switch r {
		case '/', '_':
			return '-'
		default:
			return r
		}
	}, id)

	return strings.ToLower(id)
}
