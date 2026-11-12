package gserver

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// A login attempt must never write the password to the log.
func TestValidateUserDoesNotLogPassword(t *testing.T) {
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(orig)

	const pass = "s3cret-Pa55word"
	for _, userdb := range []string{"sql", "unknown"} {
		validateUser("ana", pass, userdb, &Server{})
	}
	if strings.Contains(buf.String(), pass) {
		t.Errorf("password found in log output:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `"ana"`) {
		t.Errorf("expected the user name in the log, got:\n%s", buf.String())
	}
}
