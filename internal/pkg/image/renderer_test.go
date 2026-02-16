package image //nolint:revive // it's okay for an internal package to use this name

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestMain(m *testing.M) {
	os.Setenv("CHROME_FLAGS", "--no-sandbox")
	os.Exit(m.Run())
}

func TestRenderFailingReader(t *testing.T) {
	r := New()
	errExpected := errors.New("read failure")
	dest := &bytes.Buffer{}

	err := r.Render(dest, &failingReader{err: errExpected})
	require.Error(t, err)
	require.ErrorIs(t, err, errExpected)
	assert.Contains(t, err.Error(), "read content")
}

func TestRenderFailingWriter(t *testing.T) {
	skipIfNoBrowser(t)

	r := New()
	html := `<html><body><p>hello</p></body></html>`
	errExpected := errors.New("write failure")

	err := r.Render(&failingWriter{err: errExpected}, strings.NewReader(html))
	require.Error(t, err)
	require.ErrorIs(t, err, errExpected)
	assert.Contains(t, err.Error(), "writing screenshot")
}

func TestRenderSimpleHTML(t *testing.T) {
	skipIfNoBrowser(t)

	r := New()
	html := `<!DOCTYPE html><html><body style="background:white"><h1>Test</h1></body></html>`
	dest := &bytes.Buffer{}

	require.NoError(t, r.Render(dest, strings.NewReader(html)))

	output := dest.Bytes()
	require.NotEmpty(t, output)

	// PNG magic bytes: 0x89 P N G
	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47}
	assert.True(t, bytes.HasPrefix(output, pngMagic),
		"output does not start with PNG magic bytes, got %x", output[:min(4, len(output))])
}

func TestRenderEmptyHTML(t *testing.T) {
	skipIfNoBrowser(t)

	r := New()
	dest := &bytes.Buffer{}

	require.NoError(t, r.Render(dest, strings.NewReader("")))

	// Should still produce a valid PNG (blank page screenshot)
	pngMagic := []byte{0x89, 0x50, 0x4E, 0x47}
	assert.True(t, bytes.HasPrefix(dest.Bytes(), pngMagic),
		"expected valid PNG output even for empty HTML")
}

// helpers

type failingReader struct {
	err error
}

func (r *failingReader) Read([]byte) (int, error) {
	return 0, r.err
}

type failingWriter struct {
	err error
}

func (w *failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func skipIfNoBrowser(t *testing.T) {
	t.Helper()
	for _, name := range []string{"chromium-browser", "chromium", "google-chrome", "google-chrome-stable"} {
		if _, err := exec.LookPath(name); err == nil {
			return
		}
	}
	t.Skip("no Chrome/Chromium browser found, skipping integration test")
}
