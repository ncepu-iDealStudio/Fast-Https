package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"

	"fast-https/config"
	"fast-https/core"
	"fast-https/modules/cache"
	"fast-https/runtime/listener"
	"fast-https/runtime/request"
	"fast-https/runtime/response"
)

// TestProxyCompressEndToEnd 走完整 ProxyEvent：真实回环上游 + net.Pipe 客户端，
// 验证压缩头、长度、正文，以及缓存命中后按请求重新协商。
func TestProxyCompressEndToEnd(t *testing.T) {
	upstreamBody := []byte(strings.Repeat("proxy-e2e-body ", 120))

	t.Run("无缓存压缩", func(t *testing.T) {
		addr := startCompressUpstream(t, upstreamBody)
		cfg := &listener.ListenCfg{ProxyAddr: addr, Type: config.PROXY_HTTP, PreserveHost: true, Zip: config.ZIP_GZIP_BR}
		status, headers, body := doProxyRequest(t, cfg, "/plain", "gzip")

		if status != "200" {
			t.Fatalf("status = %q", status)
		}
		if headers["content-encoding"] != "gzip" {
			t.Fatalf("Content-Encoding = %q", headers["content-encoding"])
		}
		if headers["content-length"] != fmt.Sprint(len(body)) {
			t.Fatalf("Content-Length = %q, body = %d", headers["content-length"], len(body))
		}
		if !strings.Contains(headers["vary"], "Accept-Encoding") {
			t.Fatalf("Vary = %q", headers["vary"])
		}
		if got := gunzip(t, body); !bytes.Equal(got, upstreamBody) {
			t.Fatal("解压后正文不一致")
		}
	})

	t.Run("缓存命中按请求重新协商", func(t *testing.T) {
		addr := startCompressUpstream(t, upstreamBody)
		cacheDir := t.TempDir()
		cfg := &listener.ListenCfg{
			ProxyAddr: addr, Type: config.PROXY_HTTP, PreserveHost: true, Zip: config.ZIP_GZIP_BR,
			ProxyCache: config.Cache{
				Path: cacheDir, Valid: []string{"200:60s"},
				Key: "$request_uri", Methods: []string{"GET"}, MaxSize: 1 << 20,
			},
		}

		// 第一次未命中，gzip 压缩并写入缓存（缓存里存未压缩正文）。
		_, headers, body := doProxyRequest(t, cfg, "/cached", "gzip")
		if headers["content-encoding"] != "gzip" {
			t.Fatalf("首次响应应 gzip，实得 %q", headers["content-encoding"])
		}
		waitCacheEntry(t, cache.GetMd5("$request_uri"))

		// 命中缓存、客户端不接受压缩：返回原文，不带 Content-Encoding。
		_, headers, body = doProxyRequest(t, cfg, "/cached", "")
		if headers["content-encoding"] != "" {
			t.Fatalf("不接受的客户端不应拿到压缩体：%q", headers["content-encoding"])
		}
		if !bytes.Equal(body, upstreamBody) {
			t.Fatal("命中缓存后正文应为未压缩原文")
		}

		// 命中缓存、客户端要 br：由本机重新压缩，证明缓存不是压缩副本。
		_, headers, body = doProxyRequest(t, cfg, "/cached", "br")
		if headers["content-encoding"] != "br" {
			t.Fatalf("缓存命中应按请求选 br，实得 %q", headers["content-encoding"])
		}
		if got := unbrotli(t, body); !bytes.Equal(got, upstreamBody) {
			t.Fatal("br 解压后正文不一致")
		}
	})
}

// doProxyRequest 构造一条最小请求，调用完整 ProxyEvent，返回解析后的响应。
func doProxyRequest(t *testing.T, cfg *listener.ListenCfg, path, accept string) (string, map[string]string, []byte) {
	t.Helper()
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	ev := core.NewEvent(&listener.Listener{}, server)
	ev.RR.Req = request.RequestInit(false)
	ev.RR.Req.Method = "GET"
	ev.RR.Req.Path = path
	ev.RR.Req.Protocol = "HTTP/1.1"
	ev.RR.Req.Headers["Host"] = "api.example"
	ev.RR.Req.Headers["Connection"] = "close"
	if accept != "" {
		ev.RR.Req.Headers["Accept-Encoding"] = accept
	}
	ev.RR.ReqBuf = make([]byte, 4096)
	ev.RR.Res = response.ResponseInit()
	// 直接调 ProxyEvent 时没有运行时事件循环，按 EventWrite 的契约把响应写回连接。
	ev.EventWrite = func(e *core.Event, _ []byte) error {
		_, err := e.Conn.Write(e.ResponseBytes())
		return err
	}

	done := make(chan string, 1)
	go func() {
		raw, _ := io.ReadAll(client)
		done <- string(raw)
	}()

	ProxyEvent(cfg, ev)
	raw := <-done
	return parseProxyResponse(t, raw)
}

// startCompressUpstream 起一个只回一个固定 200 响应的回环上游。
func startCompressUpstream(t *testing.T, body []byte) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// 缓存命中时借来的连接不会被使用：靠读超时退出，避免测试结束时还挂着。
				_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
				br := bufio.NewReader(c)
				for {
					line, err := br.ReadString('\n')
					if err != nil {
						return
					}
					if strings.TrimSpace(line) == "" {
						break
					}
				}
				fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n", len(body))
				_, _ = c.Write(body)
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func parseProxyResponse(t *testing.T, raw string) (string, map[string]string, []byte) {
	t.Helper()
	head, body, ok := strings.Cut(raw, "\r\n\r\n")
	if !ok {
		t.Fatalf("响应不完整：%q", raw)
	}
	lines := strings.Split(head, "\r\n")
	parts := strings.SplitN(lines[0], " ", 3)
	if len(parts) < 2 {
		t.Fatalf("状态行非法：%q", lines[0])
	}
	headers := map[string]string{}
	for _, line := range lines[1:] {
		k, v, ok := strings.Cut(line, ":")
		if ok {
			headers[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	return parts[1], headers, []byte(body)
}

func waitCacheEntry(t *testing.T, key string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := cache.GCacheContainer.ReadCache(key); ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("缓存条目未在超时内建立")
}

func gunzip(t *testing.T, body []byte) []byte {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func unbrotli(t *testing.T, body []byte) []byte {
	t.Helper()
	out, err := io.ReadAll(brotli.NewReader(bytes.NewReader(body)))
	if err != nil {
		t.Fatal(err)
	}
	return out
}
