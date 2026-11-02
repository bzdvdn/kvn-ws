package server

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed cover_assets
var coverAssets embed.FS

var embeddedCover = func() fs.FS {
	sub, err := fs.Sub(coverAssets, "cover_assets")
	if err != nil {
		panic(err)
	}
	return sub
}()

// EmbeddedCoverFS returns the built-in cover site. It is exposed for tools
// that export the site so administrators can customize it.
func EmbeddedCoverFS() fs.FS { return embeddedCover }

// ExportCoverSite writes the embedded cover site to dst so it can be edited
// and served via fallback.dir.
func ExportCoverSite(dst string) error {
	return fs.WalkDir(embeddedCover, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out := filepath.Join(dst, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(out, 0o750)
		}
		data, rerr := fs.ReadFile(embeddedCover, p)
		if rerr != nil {
			return rerr
		}
		if merr := os.MkdirAll(filepath.Dir(out), 0o750); merr != nil {
			return merr
		}
		return os.WriteFile(out, data, 0o644) // #nosec G306 -- static public cover assets are intentionally world-readable
	})
}
