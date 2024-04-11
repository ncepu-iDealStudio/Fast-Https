package security

import (
	"encoding/base64"
	"fast-https/utils/security"
	"os"
	"path/filepath"
	"testing"
)

func TestEncrypt(t *testing.T) {
	dir, _ := os.Getwd()
	dir = filepath.Dir(filepath.Dir(dir))
	publicKeyPath := filepath.Join(filepath.Dir(filepath.Dir(dir)), "config/cert/localhost.pem")
	privateKeyPath := filepath.Join(filepath.Dir(filepath.Dir(dir)), "config/cert/localhost-key.pem")
	security.InitRSAHelper(publicKeyPath, privateKeyPath)
	var target = "123456"
	encrypt, err := security.RSAHelper.Encrypt([]byte(target))
	if err != nil {
		return
	}
	t.Log(encrypt)
	t.Log(string(encrypt))
	decrypt, err := security.RSAHelper.Decrypt(encrypt)
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
	dir = filepath.Dir(filepath.Dir(dir))
	publicKeyPath := filepath.Join(filepath.Dir(filepath.Dir(dir)), "config/cert/localhost.pem")
	privateKeyPath := filepath.Join(filepath.Dir(filepath.Dir(dir)), "config/cert/localhost-key.pem")
	security.InitRSAHelper(publicKeyPath, privateKeyPath)
	var target = "123456"
	encrypt, err := security.RSAHelper.TimeStampEncrypt(target)
	if err != nil {
		return
	}

	t.Log("encrypt base64:", base64.StdEncoding.EncodeToString(encrypt))
	decrypt, err := security.RSAHelper.TimeStampDecrypt(encrypt, 160)
	if err != nil {
		return
	}
	res := string(decrypt)
	t.Log("res:", res)
	if target != res {
		t.Error("not match")
	}
}
