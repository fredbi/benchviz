package testintegration

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/fredbi/benchviz/internal/pkg/chart"
	"github.com/fredbi/benchviz/internal/pkg/config"
	"github.com/fredbi/benchviz/internal/pkg/organizer"
	"github.com/fredbi/benchviz/internal/pkg/parser"

	"github.com/go-openapi/testify/v2/require"
)

func TestBenchviz(t *testing.T) {
	t.Run("with testify example", func(t *testing.T) {
		fixtureDir := filepath.Join("..", "..", "..", "examples", "testify")
		t.Run("should load config", func(t *testing.T) {
			cfg, err := config.Load(filepath.Join(fixtureDir, "benchviz.yaml"))
			require.NoError(t, err)
			require.NotNil(t, cfg)

			writeData(t, "test_config.json", cfg)

			t.Run("should parse benchmark", func(t *testing.T) {
				p := parser.New(cfg, parser.WithParseJSON(cfg.IsJSON))
				require.NoError(t, p.ParseFiles(filepath.Join(fixtureDir, "benchmark.json")))
				sets := p.Sets()

				writeData(t, "test_parsed.json", sets)

				t.Run("should scenarize parsed data", func(t *testing.T) {
					o := organizer.New(cfg)

					/*
						// only temporarily exported
						parsed := o.ParseBenchmarks(sets)
						require.NotEmpty(t, parsed)
						writeData(t, "test_pre_scenario.json", parsed)
					*/

					scenario := o.Scenarize(sets)
					writeData(t, "test_scenario.json", scenario)

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
