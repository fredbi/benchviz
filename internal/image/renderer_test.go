package image //nolint:revive // it's okay for an internal package to use this name

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"

	"github.com/go-openapi/testify/v2/assert"
	"github.com/go-openapi/testify/v2/require"
)

func TestRenderFailingReader(t *testing.T) {
	r := New()
	errExpected := errors.New("read failure")
	dest := &bytes.Buffer{}

	ctx, cancel := testContext(t)
	defer cancel()
	err := r.Render(ctx, dest, &failingReader{err: errExpected})
	require.Error(t, err)
	require.ErrorIs(t, err, errExpected)
	assert.Contains(t, err.Error(), "read content")
}

func TestRenderFailingWriter(t *testing.T) {
	skipIfNoBrowser(t)

	r := New()
	html := `<html><body><p>hello</p></body></html>`
	errExpected := errors.New("write failure")

	ctx, cancel := testContext(t)
	defer cancel()
	err := r.Render(ctx, &failingWriter{err: errExpected}, strings.NewReader(html))
	require.Error(t, err)
	require.ErrorIs(t, err, errExpected)
	assert.Contains(t, err.Error(), "writing screenshot")
}

func TestRenderSimpleHTML(t *testing.T) {
	skipIfNoBrowser(t)

	r := New()
	html := `<!DOCTYPE html><html><body style="background:white"><h1>Test</h1></body></html>`
	dest := &bytes.Buffer{}

	ctx, cancel := testContext(t)
	defer cancel()
	require.NoError(t, r.Render(ctx, dest, strings.NewReader(html)))

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

	ctx, cancel := testContext(t)
	defer cancel()
	require.NoError(t, r.Render(ctx, dest, strings.NewReader("")))

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

func testContext(t *testing.T) (context.Context, func()) {
	t.Helper()

	ctx := t.Context()
	if ci := os.Getenv("CI"); ci == "" {
		return ctx, func() {}
	}

	// Prepare browser options for CI/CD environments
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.DisableGPU,
		chromedp.NoSandbox, // Required for GitHub Actions and similar environments
		chromedp.Headless,
	}

	return chromedp.NewExecAllocator(ctx, opts...)
}
