package listener

import (
	"crypto/tls"
	"sync/atomic"
)

// ACMEALPNProto 是 TLS-ALPN-01 挑战约定的 ALPN 协议名（RFC 8737）。
const ACMEALPNProto = "acme-tls/1"

// tlsALPNProvider 由控制面注入：给定 ClientHello 返回挑战证书。
// 用 atomic.Pointer 整体替换函数指针，握手热路径只多一次 Load。
type tlsALPNProvider func(*tls.ClientHelloInfo) (*tls.Certificate, bool)

var tlsALPNProviderPtr atomic.Pointer[tlsALPNProvider]

// RegisterTLSALPNProvider 注册 TLS-ALPN-01 挑战证书来源，传 nil 表示撤销。
// 注册后 listenSsl 会在 ALPN 通告里追加 acme-tls/1，并让 certSet.get
// 在握手时优先问一次：控制面因此不需要自己监听 443。
func RegisterTLSALPNProvider(fn func(*tls.ClientHelloInfo) (*tls.Certificate, bool)) {
	if fn == nil {
		tlsALPNProviderPtr.Store(nil)
		return
	}
	provider := tlsALPNProvider(fn)
	tlsALPNProviderPtr.Store(&provider)
}

// TLSALPNEnabled 报告是否注册了挑战证书来源。
// listenSsl 用它决定是否通告 acme-tls/1：未启用时握手表保持不变。
func TLSALPNEnabled() bool {
	return tlsALPNProviderPtr.Load() != nil
}

// tlsALPNChallengeCert 查询挑战证书。未注册或未命中时返回 false，
// 调用方回落到正常证书。
func tlsALPNChallengeCert(hello *tls.ClientHelloInfo) (*tls.Certificate, bool) {
	fn := tlsALPNProviderPtr.Load()
	if fn == nil || hello == nil {
		return nil, false
	}
	return (*fn)(hello)
}
