package api

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func handleStatic(staticDir string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(staticDir))
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		cleanPath := filepath.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		if strings.HasPrefix(cleanPath, "/api/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		filePath := filepath.Join(staticDir, strings.TrimPrefix(cleanPath, "/"))
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			setStaticCacheHeader(w, cleanPath)
			fileServer.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(cleanPath, "/assets/") {
			if assetPath, ok := fallbackReleaseAssetPath(staticDir, strings.TrimPrefix(cleanPath, "/assets/")); ok {
				setStaticCacheHeader(w, cleanPath)
				http.ServeFile(w, r, assetPath)
				return
			}
		}

		indexPath := filepath.Join(staticDir, "index.html")
		if _, err := os.Stat(indexPath); err != nil {
			writeError(w, http.StatusNotFound, "dashboard not built")
			return
		}
		setStaticCacheHeader(w, "/index.html")
		http.ServeFile(w, r, indexPath)
	}
}

var fingerprintedAssetName = regexp.MustCompile(`-[A-Za-z0-9_-]{8}\.[A-Za-z0-9]+$`)

func setStaticCacheHeader(w http.ResponseWriter, cleanPath string) {
	if cleanPath == "/index.html" || cleanPath == "/" {
		w.Header().Set("Cache-Control", "no-store")
		return
	}
	if strings.HasPrefix(cleanPath, "/assets/") {
		if fingerprintedAssetName.MatchString(filepath.Base(cleanPath)) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// Public assets such as the favicon and OS logos keep stable names and
			// must be revalidated across releases.
			w.Header().Set("Cache-Control", "public, max-age=300, must-revalidate")
		}
	}
}

func fallbackReleaseAssetPath(staticDir string, assetName string) (string, bool) {
	cleanAssetName := filepath.Clean(assetName)
	if cleanAssetName == "." || strings.HasPrefix(cleanAssetName, "..") || filepath.IsAbs(cleanAssetName) {
		return "", false
	}

	installDir := filepath.Dir(filepath.Dir(staticDir))
	candidates, err := filepath.Glob(filepath.Join(installDir, "releases", "*", "web", "assets", cleanAssetName))
	if err != nil || len(candidates) == 0 {
		return "", false
	}
	sort.Sort(sort.Reverse(sort.StringSlice(candidates)))
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}
