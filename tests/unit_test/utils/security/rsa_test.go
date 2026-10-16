package security

import (
	"encoding/base64"
	"fast-https/utils/security"
	"path/filepath"
	"testing"
)

func setupRSAHelper(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	publicKeyPath := filepath.Join(tmp, "test-public.pem")
	privateKeyPath := filepath.Join(tmp, "test-private.pem")

	r := security.RSA{PublicKeyPath: publicKeyPath, PrivateKeyPath: privateKeyPath}
	if err := r.GenerateRSAKey(2048); err != nil {
		t.Fatalf("generate rsa key pair failed: %v", err)
	}
	security.InitRSAHelper(publicKeyPath, privateKeyPath)
}

func TestEncrypt(t *testing.T) {
	setupRSAHelper(t)
	var target = "123456"
	encrypt, err := security.RSAHelper.Encrypt([]byte(target))
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	t.Log(encrypt)
	t.Log(string(encrypt))
	decrypt, err := security.RSAHelper.Decrypt(encrypt)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	res := string(decrypt)
	t.Log(res)
	if target != res {
		t.Error("not match")
	}
}

func TestTimeStampEncrypt(t *testing.T) {
	setupRSAHelper(t)
	var target = "123456"
	encrypt, err := security.RSAHelper.TimeStampEncrypt(target)
	if err != nil {
		t.Fatalf("timestamp encrypt failed: %v", err)
	}

	t.Log("encrypt base64:", base64.StdEncoding.EncodeToString(encrypt))
	decrypt, err := security.RSAHelper.TimeStampDecrypt(encrypt, 160)
	if err != nil {
		t.Fatalf("timestamp decrypt failed: %v", err)
	}
	res := string(decrypt)
	t.Log("res:", res)
	if target != res {
		t.Error("not match")
	}
}
