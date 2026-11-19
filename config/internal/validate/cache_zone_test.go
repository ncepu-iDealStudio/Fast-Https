package validate

import (
	"strings"
	"testing"

	"fast-https/config/internal/schema"
)

func zoneServer(ref string) schema.Server {
	return schema.Server{Locations: []schema.Path{{URL: "/", ProxyCache: schema.CacheRef(ref)}}}
}

func TestValidateCacheZonesAcceptsDefinedReference(t *testing.T) {
	zones := []schema.CacheZone{{Name: "edge", Path: "./cache", Valid: []string{"200:10m", "any:30s"}}}
	unused, err := validateCacheZones(zones, []schema.Server{zoneServer("edge")})
	if err != nil {
		t.Fatal(err)
	}
	if len(unused) != 0 {
		t.Fatalf("unused = %v", unused)
	}
}

func TestValidateCacheZonesRejectsUnknownReference(t *testing.T) {
	zones := []schema.CacheZone{{Name: "edge", Path: "./cache"}}
	_, err := validateCacheZones(zones, []schema.Server{zoneServer("typo")})
	if err == nil || !strings.Contains(err.Error(), `unknown cache zone "typo"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateCacheZonesReportsUnused(t *testing.T) {
	zones := []schema.CacheZone{
		{Name: "edge", Path: "./cache"},
		{Name: "spare", Path: "./spare"},
	}
	unused, err := validateCacheZones(zones, []schema.Server{zoneServer("edge")})
	if err != nil {
		t.Fatal(err)
	}
	if len(unused) != 1 || unused[0] != "spare" {
		t.Fatalf("unused = %v", unused)
	}
}

func TestValidateCacheZonesRejectsDuplicatesAndBadFields(t *testing.T) {
	cases := []struct {
		name  string
		zones []schema.CacheZone
		want  string
	}{
		{"missing name", []schema.CacheZone{{Path: "./cache"}}, "name is required"},
		{"duplicate name", []schema.CacheZone{{Name: "edge", Path: "./a"}, {Name: "edge", Path: "./b"}}, "defined more than once"},
		{"missing path", []schema.CacheZone{{Name: "edge"}}, "path is required"},
		{"bad valid unit", []schema.CacheZone{{Name: "edge", Path: "./a", Valid: []string{"10s"}}}, "must look like"},
		{"bad status", []schema.CacheZone{{Name: "edge", Path: "./a", Valid: []string{"999:10s"}}}, "three-digit code"},
		{"zero duration", []schema.CacheZone{{Name: "edge", Path: "./a", Valid: []string{"200:0s"}}}, "duration must be positive"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := validateCacheZones(c.zones, nil)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want contains %q", err, c.want)
			}
		})
	}
}
