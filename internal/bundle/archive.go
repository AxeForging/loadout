package bundle

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// tarGz packs the contents of dir into a gzip'd tarball at destFile.
func tarGz(dir, destFile string) error {
	if err := os.MkdirAll(filepath.Dir(destFile), 0o755); err != nil {
		return err
	}
	f, err := os.Create(destFile)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(tw, src)
		return err
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// untarGz extracts an archive into dir, rejecting path traversal.
func untarGz(archive, dir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("not a gzip archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.FromSlash(hdr.Name)
		if strings.Contains(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("archive entry %q escapes the extraction dir", hdr.Name)
		}
		target := filepath.Join(dir, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode).Perm())
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil { //nolint:gosec // bundles are user-chosen local files
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		default:
			// Symlinks and specials are not part of the bundle format.
			return fmt.Errorf("unsupported archive entry type for %q", hdr.Name)
		}
	}
}

// gzipMagic is the two-byte header every gzip stream starts with.
var gzipMagic = []byte{0x1f, 0x8b}

// openBundle returns the bundle root for a directory, extracts a .tar.gz
// into a temp dir, or unpacks a single flat YAML document into a temp dir
// shaped like a bundle — detected by content, not file extension, so it
// works the same whether the path ends in .tar.gz, .yaml, or nothing at
// all. cleanup is returned for the two temp-dir cases.
func openBundle(src string) (root string, cleanup func(), err error) {
	info, err := os.Stat(src)
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() {
		return src, nil, nil
	}

	f, err := os.Open(src)
	if err != nil {
		return "", nil, err
	}
	magic := make([]byte, 2)
	n, _ := io.ReadFull(f, magic)
	f.Close()

	if n == 2 && magic[0] == gzipMagic[0] && magic[1] == gzipMagic[1] {
		tmp, err := os.MkdirTemp("", "loadout-import-*")
		if err != nil {
			return "", nil, err
		}
		if err := untarGz(src, tmp); err != nil {
			os.RemoveAll(tmp)
			return "", nil, err
		}
		return tmp, func() { os.RemoveAll(tmp) }, nil
	}

	raw, err := os.ReadFile(src)
	if err != nil {
		return "", nil, err
	}
	return unpackFlat(raw)
}
