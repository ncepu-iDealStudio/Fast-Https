// Package cfgver 把当前磁盘上的主配置与站点 JSON 做成版本快照。
// 不含证书和私钥。active 不存标志位，而是比较当前配置树哈希与各版本哈希。
package cfgver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"fast-https/config"
	"fast-https/management/state"
)

var certExt = map[string]struct{}{
	".pem": {}, ".key": {}, ".crt": {}, ".cer": {}, ".p12": {}, ".pfx": {},
}

// FileHash 是快照里一个文件的相对路径与内容哈希。
type FileHash struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Manifest 描述一个配置版本。
type Manifest struct {
	Version    string     `json:"version"`
	CreatedAt  string     `json:"created_at"`
	CreatedBy  string     `json:"created_by"`
	Summary    string     `json:"summary,omitempty"`
	Parent     string     `json:"parent,omitempty"`
	TreeSHA256 string     `json:"tree_sha256"`
	Main       string     `json:"main"`
	Files      []FileHash `json:"files"`
}

// Info 是列表里的一项。Active 由当前磁盘哈希决定。
type Info struct {
	Version   string `json:"version"`
	CreatedAt string `json:"created_at"`
	CreatedBy string `json:"created_by"`
	Summary   string `json:"summary,omitempty"`
	Parent    string `json:"parent,omitempty"`
	Active    bool   `json:"active"`
}

// ListData 是 config versions 的数据。
type ListData struct {
	Versions []Info `json:"versions"`
	Drift    bool   `json:"drift"`
}

type diskFile struct {
	Rel, Abs string
	Hash     string
}

// List 列出快照，并标出与当前磁盘配置树哈希相同的那个为 active。
func List(dir state.Dir, mainPath string) (ListData, error) {
	if _, err := os.Stat(dir.VersionsDir()); os.IsNotExist(err) {
		return ListData{Versions: []Info{}}, nil
	}
	current, err := treeHash(mainPath)
	if err != nil {
		current = ""
	}
	entries, err := os.ReadDir(dir.VersionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return ListData{Versions: []Info{}}, nil
		}
		return ListData{}, err
	}
	var versions []Info
	matched := false
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "v") {
			continue
		}
		man, err := readManifest(filepath.Join(dir.VersionsDir(), e.Name()))
		if err != nil {
			continue
		}
		active := current != "" && man.TreeSHA256 == current
		matched = matched || active
		versions = append(versions, Info{
			Version:   man.Version,
			CreatedAt: man.CreatedAt,
			CreatedBy: man.CreatedBy,
			Summary:   man.Summary,
			Parent:    man.Parent,
			Active:    active,
		})
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i].Version > versions[j].Version })
	return ListData{Versions: versions, Drift: len(versions) > 0 && !matched}, nil
}

// Snapshot 把当前配置树复制进下一个版本目录。只复制 *.json。
func Snapshot(dir state.Dir, mainPath, summary string, now time.Time) (Manifest, error) {
	release, err := dir.Lock()
	if err != nil {
		return Manifest{}, err
	}
	defer release()

	files, err := collect(mainPath)
	if err != nil {
		return Manifest{}, err
	}
	parent := latestVersion(dir)
	ver := nextVersion(parent)
	dest := filepath.Join(dir.VersionsDir(), ver)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return Manifest{}, err
	}
	_ = os.Chmod(dest, 0o700)

	man := Manifest{
		Version:   ver,
		CreatedAt: now.UTC().Format(time.RFC3339Nano),
		CreatedBy: currentUser(),
		Summary:   summary,
		Parent:    parent,
		Main:      filepath.Base(mainPath),
	}
	for _, f := range files {
		to := filepath.Join(dest, filepath.FromSlash(f.Rel))
		if err := copyFile(f.Abs, to); err != nil {
			return Manifest{}, err
		}
		man.Files = append(man.Files, FileHash{Path: f.Rel, SHA256: f.Hash})
	}
	man.TreeSHA256 = hashTree(man.Files)
	if err := writeManifest(dest, man); err != nil {
		return Manifest{}, err
	}
	return man, nil
}

// MainFile 返回某个版本里主配置文件的路径。
func MainFile(dir state.Dir, version string) (string, error) {
	ver := strings.TrimSpace(version)
	dest := filepath.Join(dir.VersionsDir(), ver)
	man, err := readManifest(dest)
	if err != nil {
		return "", err
	}
	main := man.Main
	if main == "" {
		main = "fast-https.json"
	}
	p := filepath.Join(dest, filepath.FromSlash(main))
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	return p, nil
}

func collect(mainPath string) ([]diskFile, error) {
	absMain, err := filepath.Abs(mainPath)
	if err != nil {
		return nil, err
	}
	var files []diskFile
	add := func(rel, abs string) error {
		if isCert(abs) {
			return nil
		}
		if strings.ToLower(filepath.Ext(abs)) != ".json" {
			return nil
		}
		sum, err := fileSHA(abs)
		if err != nil {
			return err
		}
		files = append(files, diskFile{Rel: filepath.ToSlash(rel), Abs: abs, Hash: sum})
		return nil
	}
	if err := add(filepath.Base(absMain), absMain); err != nil {
		return nil, err
	}
	dirs, err := config.SiteDirsFromFile(absMain)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{filepath.Base(absMain): {}}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			abs := filepath.Join(dir, e.Name())
			rel := filepath.ToSlash(filepath.Join("conf.d", e.Name()))
			if _, ok := seen[rel]; ok {
				rel = filepath.ToSlash(filepath.Join("conf.d", filepath.Base(dir)+"-"+e.Name()))
			}
			seen[rel] = struct{}{}
			if err := add(rel, abs); err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
	return files, nil
}

func treeHash(mainPath string) (string, error) {
	files, err := collect(mainPath)
	if err != nil {
		return "", err
	}
	hs := make([]FileHash, len(files))
	for i, f := range files {
		hs[i] = FileHash{Path: f.Rel, SHA256: f.Hash}
	}
	return hashTree(hs), nil
}

func hashTree(files []FileHash) string {
	sorted := append([]FileHash(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	h := sha256.New()
	for _, f := range sorted {
		fmt.Fprintf(h, "%s %s\n", f.Path, f.SHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func fileSHA(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func isCert(path string) bool {
	_, ok := certExt[strings.ToLower(filepath.Ext(path))]
	return ok
}

func copyFile(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		return err
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		_ = out.Chmod(0o600)
	}
	return out.Close()
}

func writeManifest(dir string, man Manifest) error {
	data, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(path, 0o600)
	}
	return nil
}

func readManifest(dir string) (Manifest, error) {
	var man Manifest
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	if err := json.Unmarshal(raw, &man); err != nil {
		return Manifest{}, err
	}
	return man, nil
}

func latestVersion(dir state.Dir) string {
	entries, err := os.ReadDir(dir.VersionsDir())
	if err != nil {
		return ""
	}
	latest := ""
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "v") && e.Name() > latest {
			latest = e.Name()
		}
	}
	return latest
}

func nextVersion(parent string) string {
	n := 1
	if strings.HasPrefix(parent, "v") {
		var k int
		if _, err := fmt.Sscanf(parent, "v%d", &k); err == nil {
			n = k + 1
		}
	}
	return fmt.Sprintf("v%03d", n)
}

func currentUser() string {
	if u, err := user.Current(); err == nil {
		if u.Username != "" {
			return u.Username
		}
		return u.Name
	}
	return "unknown"
}
