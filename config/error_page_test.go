package config

import (
	"encoding/json"
	"testing"
)

func TestConvertPathReadsErrorPage(t *testing.T) {
	var jp jsonPath
	err := json.Unmarshal([]byte(`{
		"url": "/",
		"type": "local",
		"error_page": {
			"not_found": "404.html",
			"proxy": "proxy.html"
		}
	}`), &jp)
	if err != nil {
		t.Fatal(err)
	}
	path := convertPath(jp)
	if path.ErrorNotFound != "404.html" || path.ErrorProxy != "proxy.html" {
		t.Fatalf("not_found=%q proxy=%q", path.ErrorNotFound, path.ErrorProxy)
	}
}
