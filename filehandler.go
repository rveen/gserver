package gserver

import (
	"net/http"
)

func FileHandler() http.HandlerFunc {
	fs := http.FileServer(http.Dir("."))

	return func(w http.ResponseWriter, r *http.Request) {
		// log doesn't seem to be enabled here
		// log.Printf("FileHandler %s\n", r.URL.Path)
		fs.ServeHTTP(w, r)
	}
}
