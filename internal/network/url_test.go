package network

import "testing"

func TestNormalizeCoreURL(t *testing.T) {
	if got := NormalizeCoreURL("192.168.1.50:9000"); got != "http://192.168.1.50:9000" {
		t.Fatalf("host:port=%q", got)
	}
	if got := NormalizeCoreURL("http://192.168.1.102"); got != "http://192.168.1.102:8090" {
		t.Fatalf("default port=%q", got)
	}
	if got := NormalizeCoreURL(""); got != DefaultCoreURL() {
		t.Fatalf("empty=%q", got)
	}
}

func TestBuildCoreURL(t *testing.T) {
	if got := BuildCoreURL("192.168.1.102", 8090); got != "http://192.168.1.102:8090" {
		t.Fatalf("build=%q", got)
	}
	host, port := ParseHostPort("http://192.168.1.102:8090")
	if host != "192.168.1.102" || port != 8090 {
		t.Fatalf("parse=%s %d", host, port)
	}
}
