package middleware

import "testing"

func TestParseBasic(t *testing.T) {
	username, password, ok := parseBasic("Basic YWRtaW46c2VjcmV0")
	if !ok || username != "admin" || password != "secret" {
		t.Fatalf("got %q %q %v", username, password, ok)
	}
	if _, _, ok := parseBasic("Bearer abc"); ok {
		t.Fatal("bearer should not parse as basic")
	}
	if _, _, ok := parseBasic("Basic not-base64"); ok {
		t.Fatal("invalid basic should fail")
	}
}
