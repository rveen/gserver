package gserver

import (
	"bytes"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/rveen/golib/fn"
	"github.com/rveen/session2"
)

// DynamicHandlerFn ...
func (srv *Server) DynamicHandlerFn(host bool, fs *fn.FNode) http.HandlerFunc {

	return func(w http.ResponseWriter, rh *http.Request) {

		t := time.Now().UnixMicro()

		// Adapt the request to gserver.Request format.
		r := ConvertRequest(rh, w, host, srv)
		if r == nil {
			// No context could be resolved for this host.
			http.Error(w, http.StatusText(500), 500)
			return
		}

		// Upload files if "UploadFiles" is present
		if rh.FormValue("UploadFiles") != "" {
			gf, _ := fileUpload(rh, "")
			data := r.Context.Node("R")
			files := data.Add("files")
			files.Add(gf)
		}

		// Get the file (or dir) corresponding to the path
		fd := *fs
		r.File = &fd
		err := r.Get()

		if err != nil {

			// Fallback to standard fylesystem
			f := *srv.Root
			r.File = &f
			err = r.Get()

			if err != nil {
				http.Error(w, http.StatusText(404), 404)
				return
			}
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

		if len(r.File.Content) == 0 {
			http.Error(w, "Empty content", 500)
		} else {
			http.ServeContent(w, rh, filepath.Base(r.Path), time.Time{}, bytes.NewReader(r.File.Content))
		}
		log.Printf("DynHandlerFn #%d %s %s %dus %s\n", session2.Len(), rh.URL.Path, rh.RemoteAddr, time.Now().UnixMicro()-t, r.Context.Node("user").String())
	}
}
