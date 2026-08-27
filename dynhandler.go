package gserver

import (
	"bytes"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/rveen/golib/fn"
	"github.com/rveen/golib/fn/httphook"
	"github.com/rveen/ogdl"
	"github.com/rveen/session2"
)

// DynamicHandler serves dynamic content from srv.Root, enforcing path-level
// auth (checkPath) and running any registered httphook interceptors.
func (srv *Server) DynamicHandler(host bool) http.HandlerFunc {
	return srv.dynamicHandler(host, nil)
}

// DynamicHandlerFn serves dynamic content from fs, falling back to srv.Root on
// a miss. A supplied fs is treated as trusted: neither checkPath nor the
// httphook interceptors run for it.
//
// Deprecated: retained for github.com/trukeio/gserver.
func (srv *Server) DynamicHandlerFn(host bool, fs *fn.FNode) http.HandlerFunc {
	return srv.dynamicHandler(host, fs)
}

func (srv *Server) dynamicHandler(host bool, fs *fn.FNode) http.HandlerFunc {

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
		if fs != nil {
			// Custom FNode: try fs first, fall back to the standard filesystem.
			fd := *fs
			r.File = &fd
			if err := r.Get(); err != nil {
				f := *srv.Root
				r.File = &f
				if err = r.Get(); err != nil {
					http.Error(w, http.StatusText(404), 404)
					return
				}
			}
		} else {
			// Check if path needs a user other than 'nobody'
			user := r.Context.Node("user").String()
			if (user == "" || user == "nobody") && !checkPath(r.Path, srv.Config) {
				http.Redirect(w, rh, "/login?redirect="+rh.URL.Path, 302)
				return
			}

			// Optional request interceptors (registered via golib/fn/httphook by
			// blank-imported adapter packages in main.go, e.g. Altium->KiCad
			// conversion). Run before normal file resolution; the first one to
			// handle the request ends processing.
			for _, h := range httphook.All() {
				if h(srv.Root, w, rh, r.Path) {
					log.Printf("DynHandler END (interceptor) %d us\n", time.Now().UnixMicro()-t)
					return
				}
			}

			// A URL naming a directory gets a trailing slash before anything
			// is served from it. Runs after the auth check above, so a 301
			// never tells an anonymous caller that a protected directory
			// exists, and after the interceptors, so a hook owning a path that
			// happens to be a directory still sees the request.
			if redirectToDirectory(w, rh, r, srv) {
				return
			}

			if err := r.Get(); err != nil {
				http.Error(w, http.StatusText(404), 404)
				return
			}
		}

		if err := r.Process(srv); err != nil {
			log.Printf("DynHandler %s: %v\n", rh.URL.Path, err)
			http.Error(w, http.StatusText(500), 500)
			return
		}

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

		// An empty body is a legitimate answer -- a zero-byte file, a template
		// that produced nothing. Process reports a genuine failure as an error
		// rather than leaving it to be guessed from the content length.
		http.ServeContent(w, rh, filepath.Base(r.Path), time.Time{}, bytes.NewReader(r.File.Content))
		log.Printf("DynHandler #%d %s %s %dus %s\n", session2.Len(), rh.URL.Path, rh.RemoteAddr, time.Now().UnixMicro()-t, r.Context.Node("user").String())

	}
}

// redirectToDirectory answers a URL that literally names a directory with a
// 301 to the same URL with a trailing slash, and reports whether it did.
// This is what http.FileServer does, and for the same reason: a browser given
// /app resolves every relative reference in the returned page against /, not
// against /app/, so <script src="bundle.js"> becomes a request for /bundle.js.
// A directory listing is the sharpest case -- every entry it links is relative.
//
// GET and HEAD only. A 301 permits a client to reissue a POST as a GET, which
// would drop the form body; gserver's templates gate writes on R.method, so
// that would quietly turn a submission into a read.
//
// The probe path and the redirect target are deliberately different strings.
// The probe uses r.Path, which is hostname-prefixed in multi-host mode; the
// target uses rh.URL.Path, since ConvertRequest runs filepath.Clean and that
// strips the trailing slash, making rh.URL.Path the only place the information
// survives.
func redirectToDirectory(w http.ResponseWriter, rh *http.Request, r *Request, srv *Server) bool {

	if rh.Method != http.MethodGet && rh.Method != http.MethodHead {
		return false
	}

	if strings.HasSuffix(rh.URL.Path, "/") {
		return false
	}

	if !srv.Root.IsDir(r.Path) {
		return false
	}

	target := rh.URL.Path + "/"
	if rh.URL.RawQuery != "" {
		target += "?" + rh.URL.RawQuery
	}

	http.Redirect(w, rh, target, http.StatusMovedPermanently)
	return true
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
