package server_test

import (
	"fast-https/config"
	"fast-https/runtime/server"
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServerInit tests server initialization
func TestServerInit(t *testing.T) {
	s := server.ServerInit()
	assert.NotNil(t, s, "Server instance should not be nil")
}

// TestScanPorts tests port scanning functionality
func TestScanPorts(t *testing.T) {
	// Test with free ports
	err := server.ScanPorts()
	assert.Nil(t, err, "Port scanning should succeed on fresh ports")

	// Test with occupied ports
	// TODO: Mock port occupation
}

// TestScanPortsReportsOccupiedPort 锁定「启动前真的检查配置里的端口」：
// 旧实现遍历 GLisinfos，而启动阶段它还是空的，等于什么都没检查。
func TestScanPortsReportsOccupiedPort(t *testing.T) {
	// 必须占用 0.0.0.0：Windows 与部分 Unix 允许再绑 127.0.0.1 上的同号端口差异，
	// 只占具体地址时通配地址仍可能绑定成功，测不出冲突。
	occupied, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err, "occupy a port for the test")
	t.Cleanup(func() { _ = occupied.Close() })

	port := strconv.Itoa(occupied.Addr().(*net.TCPAddr).Port)

	prev := config.Current()
	config.Store(&config.Fast_Https{Servers: []config.Server{{Listen: port + " "}}}, nil)
	t.Cleanup(func() { config.Store(prev, nil) })

	err = server.ScanPorts()
	require.Error(t, err, "occupied port must fail the pre-check")
	assert.Contains(t, err.Error(), port, "error should name the port")
}
