package gserver

import (
	"errors"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/icza/session"
	"github.com/rveen/ogdl"
)

var ErrZeroLength = errors.New("Zero length file")

// FileHandler returns a handler that processes all paths that exist in the file system starting
// from the root directory, whether they are static files or templates or markdown.
//
// NOTE This handler needs context information (access to Server{})
// NOTE See https://github.com/bpowers/seshcookie
// TODO serve files with http.ServeContent (handles large files with Range requests)
//
func FileHandler(srv *Server, host bool) http.Handler {

	fn := func(w http.ResponseWriter, r *http.Request) {

		// Get a session, whether or not the user has logged in
		context, user := getSession(r, w, host, srv)

		// Upload files if "UploadFiles" is present (a valid user is needed)
		if r.FormValue("UploadFiles") != "" {
			fileUpload(r, user)
		}

		// TODO is this really needed?
		// r.URL.Path = filepath.Clean(r.URL.Path)

		log.Printf("Handler %s [user %s]\n", r.URL.Path, user)

		// Get the file
		path := r.URL.Path
		if host {
			path = r.Host + "/" + path
		}
		file, err := srv.Root.Get(path, "")

		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if file == nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		// Set R.urlbase (for setting <base href="R.urlbase"> allowing relative URLs
		base := r.URL.Path
		if file.IsDir() {
			if base == "" {
				base = "/"
			} else if base[len(base)-1] != '/' {
				base += "/"
			}
		} else {
			base = filepath.Dir(base)
		}

		data := context.Node("R")
		data.Set("urlbase", base)

		// Add parameters found in the file path (_token)
		for k, v := range file.Param {
			data.Set(k, v)
		}

		context.Set("path.meta", file.Info)
		context.Set("path.data", file.Data)
		context.Set("path.content", "")

		// Process templates
		//
		// Some types have predefined templates, some ARE templates. Predefined
		// templates are taken from the main context, while the content (this
		// path) is injected into the context so that the template can pick it up.

		switch file.Type {
		case "revs":

			name := filepath.Base(file.Name)
			if name[len(name)-1] == '@' {
				name = name[:len(name)-1]
			}
			context.Set("path.filename", name)

			// Get the template used for revision lists
			tplx := context.Get("template.revs").String()
			if tplx != "" {
				// TODO preprocess templates !!
				file.Template = ogdl.NewTemplate(tplx)
				file.Content = file.Template.Process(context)
			} else {
				err = errors.New("Template not fount for type " + file.Type)
			}

		case "t":
			file.Content = file.Template.Process(context)
		case "m":
			// .md is considered a template. Here it is processed, before
			// going into the generic template.
			// TODO check this double templating thing
			context.Set("path.content", string(file.Template.Process(context)))

			tplx := ""
			if strings.HasPrefix(strings.ToLower(filepath.Base(file.Name)), "readme.") {
				tplx = context.Get("template.mddir").String()
			} else {
				tplx = context.Get("template.md").String()
			}

			if tplx != "" {
				file.Template = ogdl.NewTemplate(tplx)
				file.Content = file.Template.Process(context)
			} else {
				err = errors.New("Template not fount for type " + file.Type)
			}

		case "dir", "data/ogdl", "data/json":

			// does the tree contain a template spec?
			name := file.Data.Get("template").String()

			if name == "" {
				if file.Type == "dir" {
					name = "dir"
				} else {
					name = "data"
				}
			}
			tplx := context.Get("template." + name).String()
			if tplx != "" {
				file.Template = ogdl.NewTemplate(tplx)
				file.Content = file.Template.Process(context)
			} else {
				err = errors.New("Template not fount for type " + file.Type)
			}
		}

		// Set Content-Type (MIME type)
		if len(file.Mime) > 0 {
			w.Header().Set("Content-Type", file.Mime)
		}

		// Content-Length is set automatically in the Go http lib.
		if err != nil {
			http.Error(w, err.Error(), 500)
		} else if len(file.Content) == 0 {
			http.Error(w, "Empty content", 500)
		} else {
			w.Write(file.Content)
		}
	}

	return http.HandlerFunc(fn)
}

func fileUpload(r *http.Request, user string) error {

	if user == "nobody" || len(user) == 0 {
		return errors.New("user not logged in")
	}

	// Handle file uploads. We call ParseMultipartForm here so that r.Form[] is
	// initialized. If it isn't a multipart this gives an error.
	err := r.ParseMultipartForm(10000000) // 10M
	if err != nil {
		return err
	}

	// Where to store the file
	folder := r.FormValue("folder")
	folder = filepath.Clean(folder)
	log.Println("upload to folder", folder)

	if len(folder) > 64 || strings.Contains(folder, "..") {
		return errors.New("incorrect folder name " + folder)
	}
	folder = filepath.Clean("_user/file/" + user + "/" + folder + "/")

	os.MkdirAll(folder, 644)
	buf := make([]byte, 1000000)
	log.Println("folder for uploading:", folder)

	var file multipart.File
	var wfile *os.File
	var n int

	for k := range r.MultipartForm.File {

		vv := r.MultipartForm.File[k]

		for _, v := range vv {

			file, err = v.Open()
			if err != nil {
				return err
			}
			defer file.Close()

			log.Println("uploading:", folder+"/"+v.Filename)

			wfile, err = os.Create(folder + "/" + v.Filename)
			if err != nil {
				return err
			}
			defer wfile.Close()

			for {
				n, err = file.Read(buf)
				if n > 0 {
					wfile.Write(buf[:n])
				}
				if err != nil || n <= len(buf) {
					break
				}
			}
		}
	}

	return nil
}

func getSession(r *http.Request, w http.ResponseWriter, host bool, srv *Server) (*ogdl.Graph, string) {

	var context *ogdl.Graph

	sess := srv.Sessions.Get(r)

	// Get the context from the session, or create a new one
	if sess == nil {
		log.Println("getSession: session is new")
		sess = session.NewSession()
		srv.Sessions.Add(sess, w)

		context = ogdl.New(nil)
		if !host {
			context.Copy(srv.Context)
		} else {
			context.Copy(srv.HostContexts[r.Host])
		}
		sess.SetAttr("context", context)
		srv.ContextService.SessionContext(context, srv)
		context.Set("user", "nobody")

	} else {
		log.Println("getSession: session exists")

		context = sess.Attr("context").(*ogdl.Graph)

		if len(r.Form["_user"]) != 0 && r.Form["_user"][0] != "" {
			context.Set("user", r.Form["_user"][0])
		}
	}

	// Add request specific parameters

	data := context.Create("R")
	data.Set("url", r.URL.Path)
	data.Set("home", srv.Root.Root())

	r.ParseForm()

	// Add GET, POST, PUT parameters into context
	for k := range r.Form {
		for _, v := range r.Form[k] {
			// Check for _ogdl
			if strings.HasSuffix(k, "._ogdl") {
				k = k[0 : len(k)-6]
				g := ogdl.FromString(v)
				data.Set(k, g)
			} else {
				data.Set(k, v)
			}
		}
	}

	return context, context.Get("user").String()
}
