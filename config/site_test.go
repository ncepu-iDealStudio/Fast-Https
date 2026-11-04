package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSiteInputRoundTrip(t *testing.T) {
	in := SiteInput{
		Listens:           []string{"80", "443 ssl h2"},
		ServerName:        "blog.example",
		SSLCertificate:    "./config/cert/blog.example.pem",
		SSLCertificateKey: "./config/cert/blog.example-key.pem",
		Locations: []SiteLocation{
			{URL: "/", Type: "local", Root: "./httpdoc/root", Index: []string{"index.html"}},
			{URL: "/api", Type: "proxy", ProxyPass: "http://127.0.0.1:8080"},
		},
	}

	raw, err := in.MarshalJSONFile()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	parsed, err := ParseSiteInput(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Listens) != 2 || parsed.Listens[0] != "80" || parsed.Listens[1] != "443 ssl h2" {
		t.Fatalf("listens = %v", parsed.Listens)
	}
	if parsed.ServerName != in.ServerName {
		t.Fatalf("parsed = %+v", parsed)
	}
	if parsed.SSLCertificate != in.SSLCertificate || parsed.SSLCertificateKey != in.SSLCertificateKey {
		t.Fatalf("cert paths = %q / %q", parsed.SSLCertificate, parsed.SSLCertificateKey)
	}
	if len(parsed.Locations) != 2 || parsed.Locations[1].ProxyPass != "http://127.0.0.1:8080" {
		t.Fatalf("locations = %+v", parsed.Locations)
	}
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatal("file should end with a newline")
	}
}

func TestSiteListenNumberVsStringVsArray(t *testing.T) {
	plain, err := SiteInput{Listens: []string{"80"}, ServerName: "a.example",
		Locations: []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root"}}}.MarshalJSONFile()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plain), `"listen": 80,`) {
		t.Fatalf("plain port should be a number:\n%s", plain)
	}

	tls, err := SiteInput{Listens: []string{"443 ssl h2"}, ServerName: "a.example",
		Locations: []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root"}}}.MarshalJSONFile()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tls), `"listen": "443 ssl h2"`) {
		t.Fatalf("listen with flags should be a string:\n%s", tls)
	}

	multi, err := SiteInput{Listens: []string{"80", "443 ssl h2"}, ServerName: "a.example",
		Locations: []SiteLocation{{URL: "/", Type: "local", Root: "./httpdoc/root"}}}.MarshalJSONFile()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(multi), `"listen": [`) {
		t.Fatalf("multiple listens should be an array:\n%s", multi)
	}
	parsed, err := ParseSiteInput(multi)
	if err != nil {
		t.Fatalf("parse multi: %v", err)
	}
	if len(parsed.Listens) != 2 {
		t.Fatalf("listens = %v", parsed.Listens)
	}
}

func TestValidateSiteFileReportsFieldPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	raw := `{
  "listen": 80,
  "server_name": "bad.example",
  "location": [{ "url": "/api", "type": "proxy" }]
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	err := ValidateSiteFile(path)
	if err == nil {
		t.Fatal("proxy location without proxy_pass should be rejected")
	}
	if !strings.Contains(err.Error(), "proxy_pass") {
		t.Fatalf("error should name the field: %v", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error should name the file: %v", err)
	}
}

func TestSiteDirsFromFileAndSnapshot(t *testing.T) {
	base := t.TempDir()
	includeDir := filepath.Join(base, "conf.d")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mainPath := filepath.Join(base, "fast-https.json")
	mainConfig := `{
  "http": {
    "server": [
      { "listen": 80, "server_name": "main.example",
        "location": [{ "url": "/", "type": "local", "root": "./httpdoc/root" }] }
    ],
    "include": [` + quote(includeDir) + `]
  }
}`
	if err := os.WriteFile(mainPath, []byte(mainConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	dirs, err := SiteDirsFromFile(mainPath)
	if err != nil {
		t.Fatalf("site dirs: %v", err)
	}
	if len(dirs) != 1 || dirs[0] != includeDir {
		t.Fatalf("dirs = %v, want [%s]", dirs, includeDir)
	}

	sitePath := filepath.Join(includeDir, "blog.json")
	siteFile := `{
  "listen": "443 ssl h2",
  "server_name": "blog.example",
  "location": [{ "url": "/", "type": "local", "root": "./httpdoc/root" }]
}`
	if err := os.WriteFile(sitePath, []byte(siteFile), 0o644); err != nil {
		t.Fatal(err)
	}

	data, source, err := SnapshotServer(mainPath, "blog.example")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if source != sitePath {
		t.Fatalf("source = %q, want %q", source, sitePath)
	}
	if !strings.Contains(string(data), `"server_name": "blog.example"`) {
		t.Fatalf("snapshot body:\n%s", data)
	}

	data, source, err = SnapshotServer(mainPath, "main.example")
	if err != nil {
		t.Fatalf("snapshot main: %v", err)
	}
	if source != mainPath {
		t.Fatalf("main source = %q", source)
	}

	if _, _, err := SnapshotServer(mainPath, "missing.example"); err == nil {
		t.Fatal("unknown server_name should be reported")
	}
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(s, `\`, `\\`) + `"`
}
