package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestProcessPathReadsErrorPage(t *testing.T) {
	v := viper.New()
	v.SetConfigType("json")
	err := v.ReadConfig(strings.NewReader(`{
		"location": [{
			"url": "/",
			"type": "local",
			"error_page": {
				"not_found": "404.html",
				"proxy": "proxy.html"
			}
		}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	path := processHttpServerPath(v, "location", 0)
	if path.ErrorNotFound != "404.html" || path.ErrorProxy != "proxy.html" {
		t.Fatalf("not_found=%q proxy=%q", path.ErrorNotFound, path.ErrorProxy)
	}
}