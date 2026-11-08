package txn

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"fast-https/infra/files"
)

// FileOps 是配置文件切换用的文件系统，测试可注入失败。
type FileOps interface {
	WriteAtomic(path string, data []byte, perm os.FileMode) error
	ReadFile(path string) ([]byte, error)
	Remove(path string) error
	Rename(oldpath, newpath string) error
	MkdirAll(path string, perm os.FileMode) error
	ReadDir(path string) ([]os.DirEntry, error)
	Writable(dir string) error
}

// OSFiles 是真实文件系统。
type OSFiles struct{}

func (OSFiles) WriteAtomic(path string, data []byte, perm os.FileMode) error {
	return files.WriteAtomic(path, data, perm)
}
func (OSFiles) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (OSFiles) Remove(path string) error             { return os.Remove(path) }
func (OSFiles) Rename(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }
func (OSFiles) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}
func (OSFiles) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }
func (OSFiles) Writable(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	probe := filepath.Join(dir, ".write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return err
	}
	return os.Remove(probe)
}

func (e *Engine) files() FileOps {
	if e.Files != nil {
		return e.Files
	}
	return OSFiles{}
}

func listJSONRel(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.EqualFold(name, "manifest.json") {
			return nil
		}
		if strings.ToLower(filepath.Ext(name)) != ".json" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = path
		return nil
	})
	return out, err
}

func livePath(liveMain, rel string) string {
	if filepath.Base(rel) == filepath.Base(liveMain) && !strings.Contains(rel, "/") {
		return liveMain
	}
	return filepath.Join(filepath.Dir(liveMain), filepath.FromSlash(rel))
}

func (e *Engine) switchFiles(liveMain, candRoot, recycle string) error {
	fsys := e.files()
	cand, err := listJSONRel(candRoot)
	if err != nil {
		return err
	}
	if err := fsys.MkdirAll(filepath.Join(recycle, "conf.d"), 0o700); err != nil {
		return err
	}
	for rel, from := range cand {
		data, err := fsys.ReadFile(from)
		if err != nil {
			return err
		}
		to := livePath(liveMain, rel)
		if err := fsys.WriteAtomic(to, data, 0o644); err != nil {
			return err
		}
	}
	liveConfd := filepath.Join(filepath.Dir(liveMain), "conf.d")
	entries, err := fsys.ReadDir(liveConfd)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ent := range entries {
		if ent.IsDir() || strings.ToLower(filepath.Ext(ent.Name())) != ".json" {
			continue
		}
		rel := "conf.d/" + ent.Name()
		if _, keep := cand[rel]; keep {
			continue
		}
		src := filepath.Join(liveConfd, ent.Name())
		dst := filepath.Join(recycle, "conf.d", ent.Name())
		if err := fsys.Rename(src, dst); err != nil {
			data, rerr := fsys.ReadFile(src)
			if rerr != nil {
				return err
			}
			if werr := fsys.WriteAtomic(dst, data, 0o600); werr != nil {
				return werr
			}
			if rerr := fsys.Remove(src); rerr != nil {
				return rerr
			}
		}
	}
	return nil
}

func (e *Engine) restoreFiles(liveMain, restoreRoot string) error {
	if e.RestoreErr != nil {
		return e.RestoreErr
	}
	return e.switchFiles(liveMain, restoreRoot, filepath.Join(filepath.Dir(restoreRoot), ".restore-recycle"))
}
