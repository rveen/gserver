package gserver

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func configFile(t *testing.T, server string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "config.ogdl")
	if err := os.WriteFile(file, []byte("server\n"+server), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

// A site configured for the removed HTTPS support must fail at startup with an
// explanation, not a bare "unknown key".
func TestApplyConfigRemovedKeys(t *testing.T) {
	for _, key := range []string{"secure_host", "email", "certs"} {
		var o Options
		err := o.ApplyConfig(configFile(t, "  "+key+" x\n"))
		if err == nil || !strings.Contains(err.Error(), "no longer supported") {
			t.Errorf("server.%s: got %v, want a 'no longer supported' error", key, err)
		}
	}
}

func TestApplyConfigUnknownKey(t *testing.T) {
	var o Options
	err := o.ApplyConfig(configFile(t, "  colour blue\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown key server.colour") {
		t.Errorf("got %v, want an 'unknown key' error", err)
	}
}

func TestApplyConfigMissingFileIsNoError(t *testing.T) {
	var o Options
	if err := o.ApplyConfig(filepath.Join(t.TempDir(), "none.ogdl")); err != nil {
		t.Errorf("got %v, want nil", err)
	}
}

// Flags given on the command line win over the config file; the rest of the
// file applies, in the flags' units.
func TestFlagsWinOverConfig(t *testing.T) {
	var o Options
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	o.BindFlags(fs)
	if err := fs.Parse([]string{"-H", ":9000"}); err != nil {
		t.Fatal(err)
	}
	file := configFile(t, "  host :8080\n  timeout 30\n  session_timeout 5\n  max_sessions 7\n  userdb sql\n  log false\n  logfile x.log\n")
	if err := o.ApplyConfig(file); err != nil {
		t.Fatal(err)
	}

	if o.Host != ":9000" {
		t.Errorf("Host %q, want :9000 (the flag)", o.Host)
	}
	if o.Timeout != 30*time.Second || o.SessionTimeout != 5*time.Minute || o.MaxSessions != 7 {
		t.Errorf("Timeout %v SessionTimeout %v MaxSessions %d, want 30s 5m0s 7", o.Timeout, o.SessionTimeout, o.MaxSessions)
	}
	if o.UserDB != "sql" || !o.NoLog || o.LogFile != "x.log" {
		t.Errorf("UserDB %q NoLog %v LogFile %q, want sql true x.log", o.UserDB, o.NoLog, o.LogFile)
	}
}

func TestBindFlagsDefaults(t *testing.T) {
	// A build changes a default by setting the field first.
	o := Options{SessionTimeout: 5 * time.Minute}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	o.BindFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	if o.Host != DefaultHost || o.Timeout != DefaultTimeout || o.UserDB != DefaultUserDB || o.NoLog {
		t.Errorf("got %+v, want the library defaults", o)
	}
	if o.SessionTimeout != 5*time.Minute || fs.Lookup("ts").DefValue != "5" {
		t.Errorf("SessionTimeout %v, -ts default %q; want 5m0s, 5", o.SessionTimeout, fs.Lookup("ts").DefValue)
	}

	if err := fs.Parse([]string{"-log=false", "-t", "3", "-ts", "2"}); err != nil {
		t.Fatal(err)
	}
	if !o.NoLog || o.Timeout != 3*time.Second || o.SessionTimeout != 2*time.Minute {
		t.Errorf("NoLog %v Timeout %v SessionTimeout %v, want true 3s 2m0s", o.NoLog, o.Timeout, o.SessionTimeout)
	}
}

func TestApplyConfigBadValue(t *testing.T) {
	var o Options
	err := o.ApplyConfig(configFile(t, "  timeout soon\n"))
	if err == nil || !strings.Contains(err.Error(), "server.timeout") {
		t.Errorf("got %v, want an error naming server.timeout", err)
	}
}
