package security

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestEncrypt(t *testing.T) {
	dir, _ := os.Getwd()
	publicKeyPath := filepath.Join(filepath.Dir(filepath.Dir(dir)), "configs/rsa/public.pem")
	privateKeyPath := filepath.Join(filepath.Dir(filepath.Dir(dir)), "configs/rsa/private.pem")
	InitRSAHelper(publicKeyPath, privateKeyPath)
	var target = "123456"
	encrypt, err := RSAHelper.Encrypt([]byte(target))
	if err != nil {
		return
	}
	t.Log(encrypt)
	t.Log(string(encrypt))
	decrypt, err := RSAHelper.Decrypt(encrypt)
	if err != nil {
		return
	}

	res := string(decrypt)
	t.Log(res)
	if target != res {
		t.Error("not match")
	}
}

func TestTimeStampEncrypt(t *testing.T) {
	dir, _ := os.Getwd()
	publicKeyPath := filepath.Join(filepath.Dir(filepath.Dir(dir)), "configs/rsa/public.pem")
	privateKeyPath := filepath.Join(filepath.Dir(filepath.Dir(dir)), "configs/rsa/private.pem")
	InitRSAHelper(publicKeyPath, privateKeyPath)
	var target = "123456"
	encrypt, err := RSAHelper.TimeStampEncrypt(target)
	if err != nil {
		return
	}

	t.Log("encrypt base64:", base64.StdEncoding.EncodeToString(encrypt))
	decrypt, err := RSAHelper.TimeStampDecrypt(encrypt, 160)
	if err != nil {
		return
	}
	res := string(decrypt)
	t.Log("res:", res)
	if target != res {
		t.Error("not match")
	}
}
