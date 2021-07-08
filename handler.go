package gserver

import (
	"errors"
	"log"
	"mime"
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

		log.Printf("Handler %s [user %s]\n", r.URL.Path, user)

		// Full path
		path := r.URL.Path
		if host {
			path = r.Host + "/" + path
		}

		// process functions ('f')
		switch r.FormValue("f") {
		case "md_save":
			// save markdown file ('content' to 'path'
			content := []byte(r.FormValue("content"))
			log.Printf("Handler saving 'content' to %s (%d bytes)\n", path, len(content))
			p := srv.DocRoot + path
			if !strings.HasSuffix(p, ".md") {
				p += ".md"
			}
			err := os.WriteFile(p, content, 0666)
			if err != nil {
				log.Println(err)
			}
		}

		// Get the file
		fd := *srv.Root
		file := &fd

		var err error
		raw := false
		if r.FormValue("m") == "raw" {
			raw = true
			err = file.GetRaw(path)
		} else {
			err = file.Get(path)
		}

		if err != nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		log.Println("handler: file: ", file.Path, file.Type)

		// Set R.urlbase (for setting <base href="R.urlbase"> allowing relative URLs
		base := r.URL.Path
		if file.Type != "dir" {
			base = filepath.Dir(file.Path[len(file.Base):])
		}
		if base[len(base)-1] != '/' {
			base += "/"
		}

		data := context.Node("R")
		data.Set("urlbase", base)

		// Add parameters found in the file path (_token)
		for k, v := range file.Params {
			data.Set(k, v)
		}

		// Process templates
		//
		// Some types have predefined templates, some ARE templates. Predefined
		// templates are taken from the main context, while the content (this
		// path) is injected into the context so that the template can pick it up.

		// Process 'dir', 'file', 'document', 'data' or 'log'
		// If !raw:
		// If file, process .htm and .txt as templates
		// If dir, document or data, use corresponding template
		// When there is a readme, type is 'dir' and fn.Content is not empty

		mimeType := ""

		if !raw {
			switch file.Type {

			case "document":
				file.Content = []byte(file.Document.Html())
				fallthrough
			case "dir", "data", "log":
				context.Set("path.content", string(file.Content))
				context.Set("path.data", file.Data)

				tp := r.FormValue("t")
				if tp == "" {
					if strings.HasSuffix(file.Path, "readme.md") {
						tp = "readme"
					} else {
						tp = file.Type
					}
				}

				tpl := srv.Templates[tp]
				file.Content = tpl.Process(context)
				mimeType = "text/html"

			default: // 'file'. Check .text and .htm (templates)
				if strings.HasSuffix(file.Path, ".htm") || strings.HasSuffix(file.Path, ".text") {
					tpl := ogdl.NewTemplate(string(file.Content))
					file.Content = tpl.Process(context)
				}
			}
		} else {
			// raw content with template
			if r.FormValue("t") != "" {
				tpl := srv.Templates[r.FormValue("t")]
				file.Content = tpl.Process(context)
				mimeType = "text/html"
			}
		}

		// Set Content-Type (MIME type)
		if mimeType == "" {
			ext := filepath.Ext(file.Path)
			mimeType = mime.TypeByExtension(ext)
		}
		w.Header().Set("Content-Type", mimeType)

		// Content-Length is set automatically in the Go http lib.
		if len(file.Content) == 0 {
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
	data.Set("home", srv.Root.Base)

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
