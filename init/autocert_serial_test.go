package init

import (
	"math/big"
	"testing"
)

// randomSerialNumber：128 位随机、非负、多次调用不重复
func TestRandomSerialNumber(t *testing.T) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		s := randomSerialNumber()
		if s == nil || s.Sign() < 0 || s.Cmp(limit) >= 0 {
			t.Fatalf("serial %v out of [0, 2^128)", s)
		}
		key := s.String()
		if seen[key] {
			t.Fatalf("serial %s repeated at iteration %d", key, i)
		}
		seen[key] = true
	}
}
