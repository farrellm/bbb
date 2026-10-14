package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SPA serves the built frontend out of dir, falling back to index.html for
// any path that isn't a file, so client-side routes like /items/5 survive a
// reload. Vite's hashed /assets files are cached for good; everything else
// (index.html, the manifest, icons) is revalidated so a deploy shows up on
// the next launch.
func SPA(dir string) http.Handler {
	dir = filepath.Clean(dir)
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := resolve(dir, r.URL.Path)
		if !ok {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, index)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFile(w, r, name)
	})
}

// resolve maps a request path onto a regular file inside dir, reporting
// false when there is no such file or the path escapes dir. Directories are
// never ok: serving one would list it, and "/" must reach index.html
// through the fallback anyway.
func resolve(dir, urlPath string) (string, bool) {
	// Cleaning a rooted path drops any "..", so the join can't escape dir;
	// the prefix check is a second guard.
	name := filepath.Join(dir, filepath.Clean("/"+urlPath))
	if name != dir && !strings.HasPrefix(name, dir+string(os.PathSeparator)) {
		return "", false
	}
	info, err := os.Stat(name) //nolint:gosec // G703: confined to dir above
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	return name, true
}
