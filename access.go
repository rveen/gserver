package gserver

import (
	"errors"
	"net/http"
	"strings"
)

type LoginService struct{}

func (l LoginService) Auth(r *http.Request, s *Server) (string, error) {

	user := r.PostFormValue("User")
	// pass := r.PostFormValue("Password")

	if user != "" {
		return strings.ToLower(user), nil
	}

	return "", errors.New("not authorized")

}
