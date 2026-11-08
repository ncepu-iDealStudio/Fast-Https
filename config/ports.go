package config

import (
	"fmt"
	"os"
	"sort"
)

// ListenPortsFromFile 返回配置文件（含 http.include 展开的站点）声明的监听端口，
// 去重后升序。只读：不替换运行期快照，也不写日志。
func ListenPortsFromFile(path string) ([]int, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file failed: %w", err)
	}
	var root jsonConfig
	if err := unmarshalConfig(content, &root); err != nil {
		return nil, fmt.Errorf("parse config json failed: %w", err)
	}

	servers := append([]jsonServer(nil), root.HTTP.Servers...)
	files, err := includeFiles(root.HTTP.Include)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("read include file failed: %w", err)
		}
		var js jsonServer
		if err := unmarshalConfig(raw, &js); err != nil {
			return nil, fmt.Errorf("parse include file %s failed: %w", file, err)
		}
		servers = append(servers, js)
	}

	seen := make(map[int]struct{})
	for i := range servers {
		for _, listen := range servers[i].Listen {
			if port := listenPortOf(listen); port > 0 {
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
