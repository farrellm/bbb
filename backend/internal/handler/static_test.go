package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSPA(t *testing.T) {
	dir := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "index.html"), []byte("index"), 0o600))
	must(os.Mkdir(filepath.Join(dir, "assets"), 0o700))
	must(os.WriteFile(filepath.Join(dir, "assets", "app-abc.js"), []byte("js"), 0o600))
	must(os.WriteFile(filepath.Join(dir, "manifest.webmanifest"), []byte("{}"), 0o600))
	h := SPA(dir + "/")

	for _, tc := range []struct{ path, body, cache string }{
		{"/", "index", "no-cache"},
		{"/items/5", "index", "no-cache"},
		{"/assets/app-abc.js", "js", "public, max-age=31536000, immutable"},
		{"/manifest.webmanifest", "{}", "no-cache"},
		{"/assets/", "index", "no-cache"},
		// ServeFile itself refuses paths with "..".
		{"/../../etc/passwd", "invalid URL path\n", ""},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, nil))
		if rec.Body.String() != tc.body || rec.Header().Get("Cache-Control") != tc.cache {
			t.Errorf("%s: body %q cache %q", tc.path, rec.Body.String(), rec.Header().Get("Cache-Control"))
		}
	}
}
