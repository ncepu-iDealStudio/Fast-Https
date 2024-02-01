package safe

import (
	"fast-https/modules/safe"
	"fmt"
	"testing"
)

func TestBlackList(t *testing.T) {
	// 创建一个新的黑名单
	blacklist := safe.NewBlacklist()

	// 向黑名单中添加 IP
	blacklist.Add("127.0.0.2")
	blacklist.Add("192.168.1.2")

	// 添加 IP 段
	blacklist.Add("192.168.1.10,192.168.1.15")

	// 获取黑名单中的所有 IP 地址范围
	ipRanges := blacklist.GetIPRanges()
	fmt.Println("黑名单中的所有 IP 地址范围:")
	for _, ipRange := range ipRanges {
		fmt.Println(ipRange)
	}

	// 从黑名单中移除 IP
	blacklist.Remove("127.0.0.2")

	// 再次获取黑名单中的所有 IP 地址范围
	ipRanges = blacklist.GetIPRanges()
	fmt.Println("\n移除 IP 后的所有 IP 地址范围:")
	for _, ipRange := range ipRanges {
		fmt.Println(ipRange)
	}
}
