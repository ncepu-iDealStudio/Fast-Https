// Package cfgver 把当前磁盘上的主配置、站点 JSON 与服务证书做成快照。
// 快照按配置树哈希去重：同一棵配置树只分配一个版本号。
// ACME 存储目录（账户私钥、证书缓存）不纳入快照。
package cfgver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"fast-https/config"
	"fast-https/management/state"
)

var (
	// ErrVersionNotFound 表示版本目录不存在。
	ErrVersionNotFound = errors.New("version not found")
	// ErrActiveVersion 表示试图删除当前 active 版本。
	ErrActiveVersion = errors.New("cannot delete the active version")
)

var certExt = map[string]struct{}{
	".pem": {}, ".key": {}, ".crt": {}, ".cer": {}, ".p12": {}, ".pfx": {},
}

// FileHash 是快照里一个配置文件的相对路径与内容哈希。
type FileHash struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// CertFile 是快照里一个证书/私钥相对 config/cert 的路径与内容哈希。
type CertFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Manifest 描述一个配置版本。
type Manifest struct {
	Version    string     `json:"version"`
	CreatedAt  string     `json:"created_at"`
	CreatedBy  string     `json:"created_by"`
	Summary    string     `json:"summary,omitempty"`
	TreeSHA256 string     `json:"tree_sha256"`
	Main       string     `json:"main"`
	Files      []FileHash `json:"files"`
	Certs      []CertFile `json:"certs,omitempty"`
}

// Info 是列表里的一项。Active 由当前磁盘哈希决定。
type Info struct {
	Version     string `json:"version"`
	OperationID string `json:"operation_id,omitempty"`
	Command     string `json:"command,omitempty"`
	CreatedAt   string `json:"created_at"`
	CreatedBy   string `json:"created_by"`
	Summary     string `json:"summary,omitempty"`
	Active      bool   `json:"active"`
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
// 同一棵树存在多个历史快照时，只有版本号最大的那个算 active。
func List(dir state.Dir, mainPath string) (ListData, error) {
	if _, err := os.Stat(dir.SnapshotsDir()); os.IsNotExist(err) {
		return ListData{Versions: []Info{}}, nil
	}
	current, err := TreeHash(mainPath)
	if err != nil {
		current = ""
	}
	entries, err := os.ReadDir(dir.SnapshotsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return ListData{Versions: []Info{}}, nil
		}
		return ListData{}, err
	}
	var versions []Info
	var activeMan *Manifest
	for _, e := range entries {
		if !e.IsDir() || !IsVersionName(e.Name()) {
			continue
		}
		man, err := readManifest(filepath.Join(dir.SnapshotsDir(), e.Name()))
		if err != nil {
			continue
		}
		if current != "" && man.TreeSHA256 == current {
			if activeMan == nil || newer(man, *activeMan) {
				m := man
				activeMan = &m
			}
		}
		versions = append(versions, Info{
			Version:   man.Version,
			CreatedAt: man.CreatedAt,
			CreatedBy: man.CreatedBy,
			Summary:   man.Summary,
		})
	}
	if activeMan != nil {
		for i := range versions {
			versions[i].Active = versions[i].Version == activeMan.Version
		}
	}
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].CreatedAt == versions[j].CreatedAt {
			return versions[i].Version > versions[j].Version
		}
		return versions[i].CreatedAt > versions[j].CreatedAt
	})
	return ListData{Versions: versions, Drift: len(versions) > 0 && activeMan == nil}, nil
}

// Snapshot 把当前配置树复制进版本目录。同一棵树已存在时直接复用，
// 不新建版本；首次创建时同时保存 config/cert 下的服务证书与私钥。
func Snapshot(dir state.Dir, mainPath, summary string, now time.Time) (Manifest, error) {
	return SnapshotWithCerts(dir, mainPath, config.CERT_DIR, summary, now)
}

// SnapshotWithCerts 与 Snapshot 相同，但指定证书目录，便于测试与非默认部署。
func SnapshotWithCerts(dir state.Dir, mainPath, certDir, summary string, now time.Time) (Manifest, error) {
	release, err := dir.Lock()
	if err != nil {
		return Manifest{}, err
	}
	defer release()

	files, err := collect(mainPath)
	if err != nil {
		return Manifest{}, err
	}
	hashes := make([]FileHash, len(files))
	for i, f := range files {
		hashes[i] = FileHash{Path: f.Rel, SHA256: f.Hash}
	}
	tree := hashTree(hashes)
	if man, ok, err := findByTree(dir, tree); err != nil {
		return Manifest{}, err
	} else if ok {
		return man, nil
	}

	ver := newVersionID(dir, now)
	dest := filepath.Join(dir.SnapshotsDir(), ver)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return Manifest{}, err
	}
	_ = os.Chmod(dest, 0o700)

	man := Manifest{
		Version:    ver,
		CreatedAt:  now.UTC().Format(time.RFC3339Nano),
		CreatedBy:  currentUser(),
		Summary:    summary,
		Main:       filepath.Base(mainPath),
		Files:      hashes,
		TreeSHA256: tree,
	}
	for _, f := range files {
		to := filepath.Join(dest, filepath.FromSlash(f.Rel))
		if err := copyFile(f.Abs, to); err != nil {
			return Manifest{}, err
		}
	}
	certs, err := copyCertsInto(certDir, filepath.Join(dest, "certs"))
	if err != nil {
		return Manifest{}, err
	}
	man.Certs = certs
	if err := writeManifest(dest, man); err != nil {
		return Manifest{}, err
	}
	return man, nil
}

// HasSnapshots 报告状态目录里是否已经有版本快照。
func HasSnapshots(dir state.Dir) bool {
	entries, err := os.ReadDir(dir.SnapshotsDir())
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && IsVersionName(e.Name()) {
			return true
		}
	}
	return false
}

// VersionDir 返回某个版本快照目录。
func VersionDir(dir state.Dir, version string) string {
	return filepath.Join(dir.SnapshotsDir(), strings.TrimSpace(version))
}

// MainFile 返回某个版本里主配置文件的路径。
func MainFile(dir state.Dir, version string) (string, error) {
	ver := strings.TrimSpace(version)
	dest := filepath.Join(dir.SnapshotsDir(), ver)
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

// RestoreCerts 把某个版本快照里的证书/私钥写回 certDir，返回恢复的相对路径。
// 快照没有证书清单时返回空，不报错（兼容旧版本快照）。
func RestoreCerts(snapshotDir, certDir string) ([]string, error) {
	man, err := readManifest(snapshotDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(man.Certs) == 0 || certDir == "" {
		return nil, nil
	}
	var restored []string
	for _, c := range man.Certs {
		rel, ok := safeRel(c.Path)
		if !ok {
			return restored, fmt.Errorf("invalid cert path in snapshot: %q", c.Path)
		}
		from := filepath.Join(snapshotDir, "certs", filepath.FromSlash(rel))
		to := filepath.Join(certDir, filepath.FromSlash(rel))
		if err := copyFile(from, to); err != nil {
			return restored, err
		}
		restored = append(restored, rel)
	}
	return restored, nil
}

// BackupCerts 把 certDir 下的服务证书复制到 dest，供事务失败补偿。
func BackupCerts(certDir, dest string) error {
	_, err := copyCertsInto(certDir, dest)
	return err
}

// RestoreCertBackup 把事务开始前备份的证书复制回 certDir。
func RestoreCertBackup(backup, certDir string) error {
	files, err := collectCerts(backup)
	if err != nil {
		return err
	}
	for _, f := range files {
		rel, ok := safeRel(f.Rel)
		if !ok {
			return fmt.Errorf("invalid cert backup path: %q", f.Rel)
		}
		if err := copyFile(f.Abs, filepath.Join(certDir, filepath.FromSlash(rel))); err != nil {
			return err
		}
	}
	return nil
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
		if !config.IsTreeConfigFile(filepath.Base(abs)) {
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

// collectCerts 收集证书目录下的文件，跳过 acme/（账户私钥与证书缓存）。
func collectCerts(root string) ([]diskFile, error) {
	if root == "" {
		return nil, nil
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}
	var files []diskFile
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			if rel == "acme" {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".tmp") {
			return nil
		}
		sum, herr := fileSHA(path)
		if herr != nil {
			return herr
		}
		files = append(files, diskFile{Rel: rel, Abs: path, Hash: sum})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Rel < files[j].Rel })
	return files, nil
}

// copyCertsInto 把证书目录复制到 dest，返回写入清单。
func copyCertsInto(root, dest string) ([]CertFile, error) {
	files, err := collectCerts(root)
	if err != nil {
		return nil, err
	}
	var out []CertFile
	for _, f := range files {
		rel, ok := safeRel(f.Rel)
		if !ok {
			return nil, fmt.Errorf("invalid cert path: %q", f.Rel)
		}
		if err := copyFile(f.Abs, filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			return nil, err
		}
		out = append(out, CertFile{Path: rel, SHA256: f.Hash})
	}
	return out, nil
}

func safeRel(p string) (string, bool) {
	p = strings.TrimSpace(filepath.ToSlash(p))
	if p == "" || p == "." || p == ".." || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "../") || strings.Contains(p, "/../") {
		return "", false
	}
	if filepath.IsAbs(p) {
		return "", false
	}
	return p, true
}

// TreeHash 是当前磁盘配置树（主文件 + include 下的 json）的哈希。
func TreeHash(mainPath string) (string, error) {
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
		// 证书文件在 Windows 上可能带只读属性；回滚需要覆盖它，先清掉再试一次。
		if chmodErr := os.Chmod(to, 0o600); chmodErr == nil {
			out, err = os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		}
	}
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
	return state.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600)
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

func findByTree(dir state.Dir, tree string) (Manifest, bool, error) {
	entries, err := os.ReadDir(dir.SnapshotsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, false, nil
		}
		return Manifest{}, false, err
	}
	var found Manifest
	ok := false
	for _, e := range entries {
		if !e.IsDir() || !IsVersionName(e.Name()) {
			continue
		}
		man, err := readManifest(filepath.Join(dir.SnapshotsDir(), e.Name()))
		if err != nil || man.TreeSHA256 != tree {
			continue
		}
		if !ok || newer(man, found) {
			found = man
			ok = true
		}
	}
	return found, ok, nil
}

// newVersionID 生成 vYYYY.MM.DD.N 形式的版本号，同一天内递增；
// 已删除但仍被 operation 引用的编号不会被复用。
func newVersionID(dir state.Dir, now time.Time) string {
	date := now.Format("2006.01.02")
	prefix := "v" + date + "."
	max := 0
	bump := func(v string) {
		if !strings.HasPrefix(v, prefix) {
			return
		}
		n, err := strconv.Atoi(strings.TrimPrefix(v, prefix))
		if err == nil && n > max {
			max = n
		}
	}
	if entries, err := os.ReadDir(dir.SnapshotsDir()); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				bump(e.Name())
			}
		}
	}
	if entries, err := os.ReadDir(dir.OperationsDir()); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir.OperationsDir(), e.Name()))
			if err != nil {
				continue
			}
			var rec struct {
				Result map[string]any `json:"result"`
			}
			if json.Unmarshal(raw, &rec) != nil {
				continue
			}
			if v, ok := rec.Result["version"].(string); ok {
				bump(v)
			}
		}
	}
	return fmt.Sprintf("%s%d", prefix, max+1)
}

// IsVersionName 接受新版 vYYYY.MM.DD.N、旧版 vNNN 与迁移前的纯时间戳。
func IsVersionName(name string) bool {
	if strings.HasPrefix(name, "v") {
		rest := strings.TrimPrefix(name, "v")
		if _, err := strconv.Atoi(rest); err == nil {
			return true
		}
		parts := strings.Split(rest, ".")
		if len(parts) != 4 {
			return false
		}
		for i, p := range parts {
			if _, err := strconv.Atoi(p); err != nil {
				return false
			}
			if (i == 0 && len(p) != 4) || (i == 1 || i == 2) && len(p) != 2 {
				return false
			}
		}
		return true
	}
	return len(name) >= 19 && name[8] == '-' && name[15] == '-'
}

func newer(a, b Manifest) bool {
	if a.CreatedAt == b.CreatedAt {
		return a.Version > b.Version
	}
	return a.CreatedAt > b.CreatedAt
}

// ValidateDelete 校验版本是否可以删除：版本名合法、目录存在、不是 active。
// dry-run 预览与真实删除共用这套前置条件，避免两边判断漂移。
func ValidateDelete(dir state.Dir, version, liveMain string) error {
	version = strings.TrimSpace(version)
	if version == "" || version == "." || version == ".." || strings.ContainsAny(version, `/\`) {
		return ErrVersionNotFound
	}
	dest := filepath.Join(dir.SnapshotsDir(), version)
	info, err := os.Stat(dest)
	if err != nil || !info.IsDir() {
		return ErrVersionNotFound
	}
	list, err := List(dir, liveMain)
	if err != nil {
		return err
	}
	for _, v := range list.Versions {
		if v.Version == version && v.Active {
			return ErrActiveVersion
		}
	}
	return nil
}

// Delete 删除一个版本快照。active 版本不允许删除；operation 记录保留。
func Delete(dir state.Dir, version, liveMain string) error {
	release, err := dir.Lock()
	if err != nil {
		return err
	}
	defer release()
	if err := ValidateDelete(dir, version, liveMain); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(dir.SnapshotsDir(), strings.TrimSpace(version)))
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
