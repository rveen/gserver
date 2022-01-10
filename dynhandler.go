package gserver

import (
	"errors"
	"net/http"
)

var ErrZeroLength = errors.New("Zero length file")

// DynamicHandler ...
//
// NOTE See https://github.com/bpowers/seshcookie
// TODO serve files with http.ServeContent (handles large files with Range requests)
//
func DynamicHandler(srv *Server, host bool) http.Handler {

	fn := func(w http.ResponseWriter, rh *http.Request) {

		// Adapt the request to gserver.Request format.
		r := ConvertRequest(rh, w, host, srv)

		// Upload files if "UploadFiles" is present
		if rh.FormValue("UploadFiles") != "" {
			ff, _ := fileUpload(rh, "")
			data := r.Context.Node("R")
			files := data.Add("file")
			for _, f := range ff {
				files.Add(f)
			}
		}

		// Get the file (or dir) corresponding to the path
		err := r.Get()

		if err != nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		r.Process(srv)

		w.Header().Set("Content-Type", r.Mime)

		// Content-Length is set automatically in the Go http lib.
		if len(r.File.Content) == 0 {
			http.Error(w, "Empty content", 500)
		} else {
			w.Write(r.File.Content)
		}
	}

	return http.HandlerFunc(fn)
}
