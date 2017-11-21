package gserver

import (
	"errors"
	"net/http"
	"strings"
)

func LoginService(r *http.Request, s *Server) (string, error) {

	user := r.PostFormValue("User")
	pass := r.PostFormValue("Password")

	if pass == "" && user != "" {
		return strings.ToLower(user), nil
	}

	return "", errors.New("not authorized")

}
