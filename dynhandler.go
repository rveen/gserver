package gserver

import (
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/rveen/ogdl"
)

// DynamicHandler ...
//
// NOTE See https://github.com/bpowers/seshcookie
// TODO serve files with http.ServeContent (handles large files with Range requests)
func (srv *Server) DynamicHandler(host bool) http.HandlerFunc {

	return func(w http.ResponseWriter, rh *http.Request) {

		/*
			if rh.FormValue("UploadFiles") != "" {

			} */

		log.Println("DynHandlerFn", rh.URL.Path, rh.RemoteAddr)
		t := time.Now().UnixMicro()

		// Adapt the request to gserver.Request format.
		r := ConvertRequest(rh, w, host, srv)
		if r == nil {
			http.Error(w, "Number of open sessions exceeded", 429)
			return
		}

		// Upload files if "UploadFiles" is present
		if rh.FormValue("UploadFiles") != "" {
			gf, _ := fileUpload(rh, "")
			data := r.Context.Node("R")
			files := data.Add("files")
			files.Add(gf)
		}

		// Check if path needs a user other than 'nobody'
		user := r.Context.Node("user").String()
		if (user == "" || user == "nobody") && !checkPath(r.Path, srv.Config) {
			http.Redirect(w, rh, "/login?redirect="+rh.URL.Path, 302)
			return
		}

		// Get the file (or dir) corresponding to the path
		err := r.Get()

		if err != nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		r.Process(srv)

		w.Header().Set("Content-Type", r.Mime)

		// Content-disposition
		if rh.FormValue("filename") != "" {

			ext := filepath.Ext(r.Path)
			if ext != "" {
				fname := rh.FormValue("filename")
				fname = strings.TrimSpace(fname)
				w.Header().Set("Content-Disposition", "inline; filename=\""+fname+ext+"\"")
			}
		}

		// Content-Length is set automatically in the Go http lib.
		if len(r.File.Content) == 0 {
			http.Error(w, "Empty content", 500)
		} else {
			w.Write(r.File.Content)
		}
		log.Printf("DynHandler END %d us\n", time.Now().UnixMicro()-t)
	}
}

func checkPath(path string, cfg *ogdl.Graph) bool {

	if cfg == nil {
		return true
	}

	g := cfg.Node("allowed")

	if g != nil {
		for _, gp := range g.Out {
			p := gp.ThisString()
			if strings.HasPrefix(path, p) {
				return true
			}
		}
	}

	g = cfg.Node("protected")

	if g == nil {
		return true
	}

	for _, gp := range g.Out {
		p := gp.ThisString()
		if strings.HasPrefix(path, p) {
			return false
		}
	}
	return true
}
