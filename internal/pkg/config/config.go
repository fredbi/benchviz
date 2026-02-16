package config

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
	IsJSON      bool
	Environment string
	Render      Rendering
	Outputs     Output
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

func (c Config) GetFunction(id string) (Function, bool) {
	v, ok := c.functionIndex[id]

	return v, ok
}

func (c Config) GetContext(id string) (Context, bool) {
	v, ok := c.contextIndex[id]

	return v, ok
}

func (c Config) GetVersion(id string) (Version, bool) {
	v, ok := c.versionIndex[id]

	return v, ok
}

func (c Config) GetMetric(id MetricName) (Metric, bool) {
	v, ok := c.metricIndex[id]

	return v, ok
}

func (c Config) FindFunction(name string) (id string, ok bool) {
	for _, def := range c.Functions {
		if id, ok := def.MatchString(name); ok {
			return id, true
		}
	}

	return "", false
}

func (c Config) FindVersion(name string) (id string, ok bool) {
	for _, def := range c.Versions {
		if id, ok := def.MatchString(name); ok {
			return id, true
		}
	}

	return "", false
}

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

func (c Config) FindContext(name string) (id string, ok bool) {
	for _, def := range c.Contexts {
		if id, ok := def.MatchString(name); ok {
			return id, true
		}
	}

	return "", false
}

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

type Rendering struct {
	Title       string
	Theme       string
	Layout      Layout
	Chart       string
	Legend      LegendPosition
	Scale       Scale
	DualScale   bool
	Orientation Orientation
}

type Orientation string

const (
	OrientationVertical   Orientation = "vertical"
	OrientationHorizontal Orientation = "horizontal"
)

type File struct {
	ID        string
	MatchFile string
	Contexts  []Context
	Versions  []Version

	match *regexp.Regexp
}

func (f File) MatchString(file string) (id string, ok bool) {
	if f.match == nil {
		return "", false
	}

	if ok := f.match.MatchString(file); !ok {
		return "", false
	}

	return f.ID, true
}

type Layout struct {
	Horizontal int
	Vertical   int
}

type Scale string

const (
	ScaleAuto Scale = "auto"
	ScaleLog  Scale = "log"
)

type LegendPosition string

const (
	LegendPositionNone   LegendPosition = "none"
	LegendPositionBottom LegendPosition = "bottom"
	LegendPositionTop    LegendPosition = "top"
	LegendPositionLeft   LegendPosition = "left"
	LegendPositionRight  LegendPosition = "right"
)

type Output struct {
	HTMLFile string
	PngFile  string
	IsTemp   bool
}

type Metric struct {
	ID    MetricName
	Title string
	Axis  string
}

type Object struct {
	ID       string
	Title    string
	Match    string
	NotMatch string
	match    *regexp.Regexp
	notMatch *regexp.Regexp
}

func (o Object) Matchers() (match, notMatch *regexp.Regexp) {
	return o.match, o.notMatch
}

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

type Function struct {
	Object `mapstructure:",squash"`
}

type Context struct {
	Object `mapstructure:",squash"`
}

type Version struct {
	Object `mapstructure:",squash"`
}

type Category struct {
	ID       string
	Title    string
	Includes Includes
}

type Includes struct {
	Functions []string
	Versions  []string
	Contexts  []string
	Metrics   []MetricName
}

// Load a configuration file from the local file system.
func Load(file string) (*Config, error) {
	return load(os.DirFS(filepath.Dir(file)), filepath.Join(".", filepath.Base(file)))
}

// loadDefaults loads the default configuration from embedded FS.
func loadDefaults() (*Config, error) {
	return load(efs, "default_config.yaml")
}

func load(fsys fs.FS, file string) (*Config, error) {
	content, err := fs.ReadFile(fsys, file)
	if err != nil {
		return nil, err
	}

	var raw any
	err = yaml.Unmarshal(content, &raw)
	if err != nil {
		return nil, err
	}

	var cfg Config

	err = mapstructure.Decode(raw, &cfg)
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

	return &cfg, nil
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
