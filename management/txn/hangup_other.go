//go:build !unix

package txn

func ignoreHangup() func() { return func() {} }
