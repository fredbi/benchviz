package config

import (
	"embed"
	"errors"
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
	Functions   []Function // functions subject to meaasurements
	Contexts    []Context
	Versions    []Version
	Categories  []Category
	Files       []File // Files allows for enrichments based on the input file name

	functionIndex map[string]Function
	contextIndex  map[string]Context
	versionIndex  map[string]Version
	metricIndex   map[MetricName]Metric
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
//
// Derived contexts never match: they aggregate other contexts rather than collect
// measurements of their own.
func (c Config) FindContext(name string) (id string, ok bool) {
	for _, def := range c.Contexts {
		if def.IsDerived() {
			continue
		}

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
// Runtime-only fields (IsJSON, IsStrict, Outputs) are excluded from the output: input
// format, strictness and output paths come from the command line, and a configuration
// file that pretends to set them is misleading.
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

	dropEmptyDerived(raw, "Contexts", "DerivedContext")
	dropEmptyDerived(raw, "Categories", "DerivedCategory")

	return yaml.NewEncoder(w).Encode(raw)
}

// dropEmptyDerived removes the derived blocks that carry no aggregation formula.
//
// mapstructure has no way of omitting an empty struct, so every entry would otherwise
// advertise a "Formula:" of its own — a generated configuration should not suggest a
// feature it does not use.
func dropEmptyDerived(raw map[string]any, listKey, derivedKey string) {
	entries, ok := raw[listKey].([]map[string]any)
	if !ok {
		return
	}

	for _, fields := range entries {
		derived, ok := fields[derivedKey].(map[string]any)
		if !ok {
			continue
		}

		// the decoder keeps the value typed, but tolerate a plain string as well
		switch formula := derived["Formula"].(type) {
		case AggregationFunction:
			if formula == AggregationFunctionNone {
				delete(fields, derivedKey)
			}
		case string:
			if formula == "" {
				delete(fields, derivedKey)
			}
		}
	}
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
	// LabelFontSize sets the font size (in px) of the workload axis tick labels
	// (the per-bar category names). Zero uses the ECharts default. Reduce it when
	// long workload names overflow, typically on horizontal bar charts.
	LabelFontSize int
	Screenshot    Screenshot
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

// Context identifies a benchmark context (e.g. input size, data type, corpus) by regexp matching.
//
// Multiple Contexts for the same Function will build as many X-axis points on the chart
// (Y-axis for horizontal charts).
//
// A Context that specifies DerivedContext holds no measurement of its own: it aggregates,
// for each (function, version), the measurements of all the other (non derived) contexts
// of the category. It shows as an extra summary bar in every series, and therefore carries
// no Match or NotMatch.
//
// A configuration whose contexts are all derived is invalid: there would be nothing to
// aggregate.
type Context struct {
	Object `mapstructure:",deep,squash"`

	DerivedContext Derived
}

// IsDerived reports whether the context aggregates other contexts instead of holding
// measurements of its own.
func (c Context) IsDerived() bool {
	return c.DerivedContext.IsDerived()
}

// Version identifies a benchmark implementation variant (e.g. "reflect", "generics") by regexp matching.
//
// Multiple Versions will build as many bar series, side by side on each X-axis point.
type Version struct {
	Object `mapstructure:",deep,squash"`
}

// Derived turns a [Context] or a [Category] into an aggregate of actual measurements.
//
// When Formula is set to another value than [AggregationFunctionNone], that
// [AggregationFunction] is applied over the measurements of the entries that are not
// themselves derived — aggregation never applies to another derived series.
//
// Derived series are always evaluated last, even if they appear early in the list: their
// position in the configuration only decides where they show up in the layout.
type Derived struct {
	Formula AggregationFunction
}

// IsDerived reports whether an aggregation function is set.
func (d Derived) IsDerived() bool {
	return d.Formula != AggregationFunctionNone
}

// AggregationFunction specifies how measurements are aggregated to construct a derived series.
type AggregationFunction string

const (
	AggregationFunctionNone    AggregationFunction = ""
	AggregationFunctionMean    AggregationFunction = "mean"
	AggregationFunctionGeoMean AggregationFunction = "geomean"
	AggregationFunctionMax     AggregationFunction = "max"
	AggregationFunctionMin     AggregationFunction = "min"
)

func (f AggregationFunction) String() string {
	return string(f)
}

// IsValid reports whether the aggregation function is one of the supported formulas.
//
// [AggregationFunctionNone] is not valid: it merely signals a non derived entry.
func (f AggregationFunction) IsValid() bool {
	switch f {
	case AggregationFunctionMean, AggregationFunctionGeoMean, AggregationFunctionMax, AggregationFunctionMin:
		return true
	default:
		return false
	}
}

// AllAggregationFunctions returns all the supported aggregation formulas.
func AllAggregationFunctions() []AggregationFunction {
	return []AggregationFunction{
		AggregationFunctionMean,
		AggregationFunctionGeoMean,
		AggregationFunctionMax,
		AggregationFunctionMin,
	}
}

// Category groups functions, contexts, versions and metrics into a single chart.
//
// (Function,Context,Version) corresponds to a single data point for a Metric.
//
// A DerivedCategory may be specified: this one aggregates over Contexts and Versions,
// producing for each Function a single aggregated measurement — a simplified chart
// conveying the bottom line of the other categories.
//
// A Category that specifies DerivedCategory may leave its Includes clause out: whatever
// it does not state is implied from the other (non derived) categories. Stating a
// dimension explicitly narrows the aggregate, which is how workloads that are not really
// comparable are kept out of the same bottom line.
//
// A configuration whose categories are all derived is invalid: there would be nothing to
// aggregate.
//
// TODO: optional markpoints (min,max), optional styled single bar
// TODO: add "baseline" property so all other series are relative to the baseline.
type Category struct {
	ID              string
	Title           string
	Includes        Includes
	DerivedCategory Derived
}

// IsDerived reports whether the category aggregates the other categories instead of
// plotting measurements of its own.
func (c Category) IsDerived() bool {
	return c.DerivedCategory.IsDerived()
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
	var derived int

	for i, v := range c.Contexts {
		if v.ID == "" {
			return fmt.Errorf("invalid contexts: empty ID found: contexts[%d]", i)
		}
		if _, ok := c.contextIndex[v.ID]; ok {
			return fmt.Errorf("invalid contexts: duplicate ID key found: %s", v.ID)
		}

		if v.IsDerived() {
			derived++
			location := fmt.Sprintf("contexts[%d] (%s)", i, v.ID)

			if err := validateDerived(v.DerivedContext, location); err != nil {
				return err
			}

			// A derived context holds no measurement of its own, so it may not carry a
			// matching rule: would it match a benchmark, actual measurements would land
			// in the aggregate.
			if v.Match != "" || v.NotMatch != "" {
				return fmt.Errorf("invalid derived %s: a derived context aggregates other contexts and may not define match or notMatch", location)
			}

			if v.Title == "" {
				// the formula names the summary bar (e.g. "ReadJSON - Geomean")
				v.Title = titleize(v.DerivedContext.Formula)
			}
		}

		if v.Title == "" {
			v.Title = titleize(v.ID)
		}
		c.contextIndex[v.ID] = v
	}

	if derived > 0 && derived == len(c.Contexts) {
		return errors.New("invalid contexts: all contexts are derived: there is no measurement left to aggregate")
	}

	return nil
}

// validateDerived checks the aggregation settings shared by all derived entries.
func validateDerived(derived Derived, location string) error {
	if !derived.Formula.IsValid() {
		return fmt.Errorf("invalid derived %s: unknown aggregation formula %q (should be one of %v)",
			location, derived.Formula, AllAggregationFunctions())
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
	var derived int

	for i, v := range c.Categories {
		if v.IsDerived() {
			derived++

			v, err = c.validateDerivedCategory(v, i)
		} else {
			v, err = c.validateCategory(v, i)
		}
		if err != nil {
			return err
		}

		c.Categories[i] = v
	}

	if derived > 0 && derived == len(c.Categories) {
		return errors.New("invalid categories: all categories are derived: there is no measurement left to aggregate")
	}

	return nil
}

// validateDerivedCategory validates a category that aggregates the other ones.
//
// Each dimension of its Includes clause is optional: whatever it leaves out is implied
// from the non derived categories. Narrowing it explicitly is how one keeps workloads
// that are not really comparable out of the same aggregate.
//
// Unlike a regular category, nothing is injected here: the implied scope is only known
// once every category has been validated, so it is resolved when the series are built.
func (c *Config) validateDerivedCategory(v Category, i int) (vv Category, err error) {
	if v.ID == "" {
		return vv, fmt.Errorf("invalid categories: empty ID found: categories[%d]", i)
	}

	if err := validateDerived(v.DerivedCategory, fmt.Sprintf("categories[%d] (%s)", i, v.ID)); err != nil {
		return vv, err
	}

	if v.Title == "" {
		v.Title = "{metric} - " + titleize(v.DerivedCategory.Formula)
	}

	if err := c.validateIncludeRefs(v); err != nil {
		return vv, err
	}

	// an aggregate never feeds another aggregate
	for j, ref := range v.Includes.Contexts {
		if context, ok := c.contextIndex[ref]; ok && context.IsDerived() {
			return vv, fmt.Errorf("invalid derived category: categories.%s.includes.contexts[%d]=%s is itself derived: an aggregate may not aggregate another one", v.ID, j, ref)
		}
	}

	return v, nil
}

// validateIncludeRefs checks that every ID referenced by a category is defined.
func (c *Config) validateIncludeRefs(v Category) error {
	includes := v.Includes

	for j, ref := range includes.Functions {
		if _, ok := c.functionIndex[ref]; !ok {
			return fmt.Errorf("invalid category: function ID not found categories.%s.includes.functions[%d]=%s", v.ID, j, ref)
		}
	}

	for j, ref := range includes.Contexts {
		if _, ok := c.contextIndex[ref]; !ok {
			return fmt.Errorf("invalid category: context ID not found categories.%s.includes.contexts[%d]=%s", v.ID, j, ref)
		}
	}

	for j, ref := range includes.Versions {
		if _, ok := c.versionIndex[ref]; !ok {
			return fmt.Errorf("invalid category: version ID not found categories.%s.includes.versions[%d]=%s", v.ID, j, ref)
		}
	}

	for j, ref := range includes.Metrics {
		if _, ok := c.metricIndex[ref]; !ok {
			return fmt.Errorf("invalid category: metric ID not found categories.%s.includes.metrics[%d]=%s", v.ID, j, ref)
		}
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

	var derivedContexts int
	for j, ref := range includes.Contexts {
		context, ok := c.contextIndex[ref]
		if !ok {
			return vv, fmt.Errorf("invalid category: context ID not found categories.%s.includes.contexts[%d]=%s", v.ID, j, ref)
		}

		if context.IsDerived() {
			derivedContexts++
		}
	}

	if derivedContexts > 0 && derivedContexts == len(includes.Contexts) {
		return vv, fmt.Errorf("invalid category: all contexts of categories.%s.includes.contexts are derived: there is no measurement left to aggregate", v.ID)
	}

	if len(includes.Contexts) == 0 {
		// Derived contexts are never injected implicitly: an extra summary bar is opt-in,
		// per category.
		for _, injected := range c.Contexts {
			if injected.IsDerived() {
				continue
			}

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
