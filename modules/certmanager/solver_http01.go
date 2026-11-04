package certmanager

import (
	"context"
	"errors"
	"fast-https/core"
	"fast-https/infra/logging"
	"fast-https/runtime/response"
	"strconv"
	"strings"
	"sync"

	"github.com/mholt/acmez/v3/acme"
)

// http01ChallengePrefix 是 RFC 8555 §8.3 规定的应答路径前缀。
const http01ChallengePrefix = "/.well-known/acme-challenge/"

// http01Solver 把 http-01 的 token 交给数据面应答：Present 写内存表，
// 数据面在 location 匹配之前按路径查表返回 keyAuthorization。
// token 只在一次挑战期间有效，所以 CleanUp 必须删干净。
type http01Solver struct {
	mu     sync.RWMutex
	tokens map[string]string
}

func newHTTP01Solver() *http01Solver {
	return &http01Solver{tokens: make(map[string]string)}
}

// Present 实现 acmez.Solver。
func (s *http01Solver) Present(_ context.Context, chal acme.Challenge) error {
	if chal.Token == "" {
		return errors.New("acme: http-01 challenge without token")
	}
	s.mu.Lock()
	s.tokens[chal.Token] = chal.KeyAuthorization
	s.mu.Unlock()
	logging.ConsoleLog.Debug().Msgf("acme: http-01 token ready for %s", chal.Identifier.Value)
	return nil
}

// CleanUp 实现 acmez.Solver。
func (s *http01Solver) CleanUp(_ context.Context, chal acme.Challenge) error {
	s.mu.Lock()
	delete(s.tokens, chal.Token)
	s.mu.Unlock()
	return nil
}

func (s *http01Solver) lookup(token string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.tokens[token]
	return value, ok
}

// handle 在数据面应答一次挑战请求，返回 true 表示响应已经写好。
// token 不在表内时返回 false：不能劫持站点自己托管的
// /.well-known/acme-challenge/（比如用户用别的客户端做 webroot 校验）。
func (s *http01Solver) handle(ev *core.Event) bool {
	if ev == nil || ev.RR.Req == nil {
		return false
	}
	path := ev.RR.Req.Path
	if !strings.HasPrefix(path, http01ChallengePrefix) {
		return false
	}
	token := path[len(http01ChallengePrefix):]
	// 挑战 token 是单段路径；带斜杠的请求不是 ACME 发出的。
	if token == "" || strings.Contains(token, "/") {
		return false
	}
	keyAuth, ok := s.lookup(token)
	if !ok {
		return false
	}

	res := ev.RR.Res
	if res == nil {
		res = response.ResponseInit()
		ev.RR.Res = res
	}
	res.SetFirstLine(200, "OK")
	res.SetHeader("Server", "Fast-Https")
	res.SetHeader("Date", response.CachedDate())
	res.SetHeader("Content-Type", "text/plain")
	res.SetHeader("Content-Length", strconv.Itoa(len(keyAuth)))
	res.SetBody([]byte(keyAuth))
	ev.WriteResponseClose(nil)
	logging.ConsoleLog.Debug().Msgf("acme: answered http-01 challenge for %s", token)
	return true
}
