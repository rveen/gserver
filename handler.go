package gserver

// Deprecated!!

import (
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rveen/ogdl"
)

// Deprecated: FileHandler returns a handler that processes all paths that exist in the file system starting
// from the root directory, whether they are static files or templates or markdown.
//
// NOTE This handler needs context information (access to Server{})
// NOTE See https://github.com/bpowers/seshcookie
// TODO serve files with http.ServeContent (handles large files with Range requests)
//
func FileHandler_(srv *Server, host bool) http.Handler {

	fn := func(w http.ResponseWriter, r *http.Request) {

		// Get a session, whether or not the user has logged in
		context := getSession(r, w, host, srv)
		user := context.Get("user").String()

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

		// Set R.urlbase (for setting <base href="$R.urlbase"> allowing relative URLs)
		base := r.URL.Path
		if file.Type != "dir" {
			base = filepath.Dir(file.Path[len(file.Root):])
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

					context.Set("mime", "")
					file.Content = tpl.Process(context)

					// Allow templates to return arbitrary mime types
					mime := context.Get("mime").String()
					if mime != "" {
						mimeType = mime
						file.Content = context.Get("content").Binary()
					}
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
