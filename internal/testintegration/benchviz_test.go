package testintegration

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/fredbi/benchviz/internal/chart"
	"github.com/fredbi/benchviz/internal/config"
	"github.com/fredbi/benchviz/internal/organizer"
	"github.com/fredbi/benchviz/internal/parser"

	"github.com/go-openapi/testify/v2/require"
)

func TestBenchviz(t *testing.T) {
	t.Run("with testify example", func(t *testing.T) {
		fixtureDir := filepath.Join("..", "..", "examples", "testify")
		t.Run("should load config", func(t *testing.T) {
			cfg, err := config.Load(filepath.Join(fixtureDir, "benchviz.yaml"))
			require.NoError(t, err)
			require.NotNil(t, cfg)

			writeData(t, "test_config.json", cfg)

			t.Run("should parse benchmark", func(t *testing.T) {
				// The fixture is JSON. cfg.IsJSON is a runtime-only field fed by the
				// -json CLI flag (it is not unmarshalled from the yaml), so it must be
				// set explicitly here: reading the fixture as plain text silently
				// yields an empty set and leaves the whole pipeline untested.
				p := parser.New(cfg, parser.WithParseJSON(true))
				require.NoError(t, p.ParseFiles(filepath.Join(fixtureDir, "benchmark.json")))
				sets := p.Sets()
				require.NotEmpty(t, sets)

				writeData(t, "test_parsed.json", sets)

				t.Run("should scenarize parsed data", func(t *testing.T) {
					o := organizer.New(cfg)

					/*
						// only temporarily exported
						parsed := o.ParseBenchmarks(sets)
						require.NotEmpty(t, parsed)
						writeData(t, "test_pre_scenario.json", parsed)
					*/

					scenario, err := o.Scenarize(sets)
					require.NoError(t, err)
					writeData(t, "test_scenario.json", scenario)

					require.NotEmpty(t, scenario.Categories)
					for _, category := range scenario.Categories {
						require.NotEmpty(t, category.XLabels, "category %q has no workload axis", category.ID)

						for _, data := range category.Data {
							for _, series := range data.Series {
								// series data is mapped to the workload axis by index
								require.Len(t, series.Points, len(category.XLabels),
									"series %q of category %q is not aligned with the workload axis",
									series.Title, category.ID)
							}
						}
					}

					t.Run("should build page", func(t *testing.T) {
						builder := chart.New(cfg, scenario)
						page := builder.BuildPage()

						writeData(t, "test_page.json", page)
						t.Run("should render page", func(t *testing.T) {
							var buf bytes.Buffer
							require.NoError(t, page.Render(&buf))

							writeResult(t, "test_html.html", &buf)
						})
					})
				})
			})
		})
	})
}

func writeData(t *testing.T, name string, data any) {
	t.Helper()

	buf, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)

	rdr := bytes.NewReader(buf)
	writeResult(t, name, rdr)
}

func writeResult(t *testing.T, name string, rdr io.Reader) {
	t.Helper()

	file, err := os.Create(name)
	require.NoError(t, err)

	_, err = io.Copy(file, rdr)
	require.NoError(t, err)
}
