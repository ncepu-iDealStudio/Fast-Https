// Package scan 只读地扫描配置树：监听端口与证书声明。不替换运行期快照。
package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"fast-https/config/internal/model"
	"fast-https/config/internal/paths"
	"fast-https/config/internal/schema"
	"fast-https/config/internal/source"
)

// ListenPortsFromFile 返回配置文件（含 http.include 展开的站点）声明的监听端口，
// 去重后升序。只读：不替换运行期快照，也不写日志。
func ListenPortsFromFile(path string) ([]int, error) {
	return listenPortsAt(path, model.FAST_HTTPS_ETC_DIR, "", ".")
}

// ListenPortsFromTree 按候选目录解析 include，不读线上 conf.d。
func ListenPortsFromTree(root string) ([]int, error) {
	abs, err := paths.ResolveCandidateRoot(root)
	if err != nil {
		return nil, err
	}
	main := filepath.Join(abs, "fast-https.json")
	if _, err := os.Stat(main); err != nil {
		return nil, err
	}
	return listenPortsAt(main, abs, abs)
}

func listenPortsAt(path, includeBase, treeRoot string, fallbacks ...string) ([]int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file failed: %w", err)
	}
	var root schema.Config
	if err := schema.Unmarshal(content, &root); err != nil {
		return nil, fmt.Errorf("parse config json failed: %w", err)
	}

	servers := append([]schema.Server(nil), root.HTTP.Servers...)
	files, err := source.IncludeFiles(root.HTTP.Include, includeBase, treeRoot, fallbacks...)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read include file failed: %w", err)
		}
		var js schema.Server
		if err := schema.Unmarshal(raw, &js); err != nil {
			return nil, fmt.Errorf("parse include file %s failed: %w", file, err)
		}
		servers = append(servers, js)
	}

	seen := make(map[int]struct{})
	for i := range servers {
		for _, listen := range servers[i].Listen {
			if port := schema.ListenPort(listen); port > 0 {
				seen[port] = struct{}{}
			}
		}
	}
	ports := make([]int, 0, len(seen))
	for port := range seen {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports, nil
}
