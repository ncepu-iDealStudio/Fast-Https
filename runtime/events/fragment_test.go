package events

import (
	"net"
	"testing"
	"time"

	"fast-https/core"
	"fast-https/runtime/filters"
	"fast-https/runtime/listener"
)

// 请求头分片到达：真实的 TCP 从不保证"头一次读全"。这里把一个合法
// 请求拆成 8 段（含把 CRLF 切在 \r 与 \n 之间、把头值切半），验证
// parseRequest 的"读完整或判非法为止"循环能逐段拼回，而不是把首个
// 半截缓冲误判成 400。
//
// 回归背景：修复前 ParseHeader 对"缓冲里还没有 CRLF"一律返回
// UnknowInvalid，首段只要不含完整行（例如只有 "G"）就会被 400 拒绝。
func TestParseRequestHeaderArrivingFragmented(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	go func() {
		fragments := []string{
			"G",                      // 方法残片
			"ET /f",                  // 方法尾 + 路径头
			"rag HTTP/",              // 路径尾 + 协议头
			"1.1\r\n",                // 协议尾 + 行终止
			"Host: f",                // 头名 + 值头
			"rag.example\r",          // 值尾 + 半个 CRLF（\n 在下一段）
			"\nAccept: text/plain\r", // 补上 \n；下一个头再断在 \r
			"\n",                     // 补齐上一个头的 \n
			"\r\n",                   // 终止空行
		}
		for _, frag := range fragments {
			if _, err := client.Write([]byte(frag)); err != nil {
				return
			}
		}
		// 保持写端打开直到服务端读完，避免 pipe 提前 EOF 干扰断言
		time.Sleep(200 * time.Millisecond)
	}()

	ev := core.NewEvent(&listener.Listener{}, server)
	fif := filters.NewFilter()

	done := make(chan int, 1)
	go func() { done <- parseRequest(ev, fif) }()

	select {
	case code := <-done:
		if code != 1 {
			t.Fatalf("parseRequest() = %d, want 1 (parsed ok)", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parseRequest never returned; fragmented header wedged the read loop")
	}

	req := ev.RR.Req
	if req.Method != "GET" || req.Path != "/frag" || req.Protocol != "HTTP/1.1" {
		t.Fatalf("request line = %s %s %s", req.Method, req.Path, req.Protocol)
	}
	if req.GetHost() != "frag.example" {
		t.Fatalf("Host = %q", req.GetHost())
	}
	if req.GetHeader("Accept") != "text/plain" {
		t.Fatalf("Accept = %q", req.GetHeader("Accept"))
	}
}

// 分片到达的恶意形态：裸 LF 分行的请求在分段喂入时也必须被拒，
// 而不是"凑够完整头就放行"。parseRequest 应返回 0（连接终结）。
func TestParseRequestRejectsFragmentedBareLF(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { _ = client.Close() })

	// 恶意形态只喂一段就足以触发拒绝；后续分片写在独立协程，
	// 排空响应在另一协程，避免无缓冲管道上互相卡死
	go func() {
		_, _ = client.Write([]byte("GET / HTTP/1.1\n"))
	}()
	go func() {
		_, _ = client.Write([]byte("Host: a\n"))
		_, _ = client.Write([]byte("\n"))
	}()
	go func() {
		buf := make([]byte, 512)
		client.SetReadDeadline(time.Now().Add(2 * time.Second))
		for {
			if _, err := client.Read(buf); err != nil {
				return
			}
		}
	}()

	ev := core.NewEvent(&listener.Listener{}, server)
	ev.EventWrite = EventWrite // 生产流程在 HandleEvent 装配
	fif := filters.NewFilter()

	done := make(chan int, 1)
	go func() { done <- parseRequest(ev, fif) }()

	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("parseRequest() = %d, want 0 (rejected)", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("parseRequest never returned")
	}
}
