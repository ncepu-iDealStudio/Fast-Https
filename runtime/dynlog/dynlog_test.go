package dynlog

import (
	"testing"

	"fast-https/config"
)

func TestPathTypeName(t *testing.T) {
	cases := map[uint16]string{
		config.LOCAL:      "LOCAL",
		config.PROXY_HTTP: "PROXY_HTTP",
		config.PROXY_HTTPS: "PROXY_HTTPS",
		config.PROXY_TCP:  "PROXY_TCP",
		config.REWRITE:    "REWRITE",
		99:                "",
	}
	for typ, want := range cases {
		if got := pathTypeName(typ); got != want {
			t.Errorf("pathTypeName(%d) = %q, want %q", typ, got, want)
		}
	}
}

// fieldTime 是 zerolog 时间戳的兼容占位：调用本身不得 panic
func TestFieldTimeIsNoop(t *testing.T) {
	fieldTime(nil, nil, nil)
}
