package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A site configured for the removed HTTPS support must fail at startup with an
// explanation, not a bare "unknown key".
func TestApplyConfigRemovedKeys(t *testing.T) {
	for _, key := range []string{"secure_host", "email", "certs"} {
		file := filepath.Join(t.TempDir(), "config.ogdl")
		if err := os.WriteFile(file, []byte("server\n  "+key+" x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := applyConfig(file)
		if err == nil || !strings.Contains(err.Error(), "no longer supported") {
			t.Errorf("server.%s: got %v, want a 'no longer supported' error", key, err)
		}
	}
}

func TestApplyConfigUnknownKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.ogdl")
	if err := os.WriteFile(file, []byte("server\n  colour blue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := applyConfig(file)
	if err == nil || !strings.Contains(err.Error(), "unknown key server.colour") {
		t.Errorf("got %v, want an 'unknown key' error", err)
	}
}
