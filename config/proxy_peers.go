package config

// ProxyPeerGroups 返回每个代理 location 的上游地址（host:port）。
func ProxyPeerGroups(path string) ([][]string, error) {
	cfg, err := processRoot(path)
	if err != nil {
		return nil, err
	}
	var groups [][]string
	for _, server := range cfg.Servers {
		for _, path := range server.Path {
			if path.PathType != PROXY_HTTP && path.PathType != PROXY_HTTPS {
				continue
			}
			if len(path.ProxyPeers) == 0 {
				continue
			}
			groups = append(groups, append([]string(nil), path.ProxyPeers...))
		}
	}
	return groups, nil
}
