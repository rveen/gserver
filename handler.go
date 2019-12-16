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
	"github.com/rveen/golib/fs/sysfs"
	"github.com/rveen/ogdl"
)

var ErrZeroLength = errors.New("Zero length file")

// FileHandler returns a handler that processes all paths that exist in the file system starting
// from the root directory, whether they are static files or templates or markdown.
//
// NOTE This handler is a final one. If the path doesn't exist, it returns 'Not found'
// NOTE This handler needs context information (access to Server{})
// NOTE See https://github.com/bpowers/seshcookie
// TODO serve files with http.ServeContent (handles large files with Range requests)
//
func FileHandler(srv *Server) http.Handler {

	fn := func(w http.ResponseWriter, r *http.Request) {

		// Get the user from the form. This is set in loginHandler()
		user := r.FormValue("user")

		// Get a session, whether or not the user has logged in
		sess := srv.Sessions.Get(r)
		if sess == nil {
			sess = session.NewSession()
			sess.SetAttr("user", user)
			srv.Sessions.Add(sess, w)
		}

		// Upload files if "UploadFiles" is present (a session and valid user are needed)

		if user != "nobody" && r.FormValue("UploadFiles") != "" {

			// Handle file uploads. We call ParseMultipartForm here so that r.Form[] is
			// initialized. If it isn't a multipart this gives an error that we are ignoring.
			err := r.ParseMultipartForm(10000000) // 10M

			for {
				if err != nil {
					break
				}

				// Where to store the file
				folder := r.FormValue("folder")
				folder = filepath.Clean(folder)
				log.Println("upload to folder", folder)

				if len(folder) > 64 || strings.Contains(folder, "..") {
					break
				}

				// Without authenticated user no upload is possible
				user := sess.Attr("user").(string)
				if user == "nobody" || len(user) == 0 {
					break
				}

				folder = "_user/file/" + user + "/" + folder + "/"

				// Prepare and clean path
				folder = filepath.Clean(folder)

				os.MkdirAll(folder, 644)
				buf := make([]byte, 1000000)
				log.Println("Folder created", folder)

				var file multipart.File
				var wfile *os.File
				var n int

				for k := range r.MultipartForm.File {

					vv := r.MultipartForm.File[k]

					for _, v := range vv {
						//data.Set(k, v.Filename)

						file, err = v.Open()
						if err != nil {
							//err2 = err
							continue
						}

						log.Println("uploaded", folder+"/"+v.Filename)
						wfile, err = os.Create(folder + "/" + v.Filename)
						if err != nil {
							file.Close()
							//err2 = err
							continue
						}

						for {
							n, err = file.Read(buf)
							if n > 0 {
								wfile.Write(buf[:n])
							}
							if err != nil || n <= len(buf) {
								break
							}
						}

						wfile.Close()
						file.Close()
					}
				}

				break
			}
		}

		// Create the context (early because of files.Get)
		var context *ogdl.Graph
		i := sess.Attr("context")

		if i == nil {
			context = ogdl.New(nil)
			context.Copy(srv.Context)
			sess.SetAttr("context", context)
			srv.ContextService.Load(context, srv)
		} else {
			context = i.(*ogdl.Graph)
		}
		context.Set("user", user)
		context.Substitute("$_user", user)

		data := context.Create("R")
		url := filepath.Clean(r.URL.Path)
		data.Set("url", url)
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

		// Get the file
		file, err := sysfs.Get(srv.Root, url, "")

		if file == nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		// Set R.urlbase (for setting <base href="R.urlbase"> allowing relative URLs
		st, err := os.Stat(srv.Root.Root() + "/" + url)
		if err == nil && !st.IsDir() {
			// Remove the file part
			url = filepath.Dir(url)
		}
		if len(url) != 0 && url[len(url)-1] != '/' {
			url += "/"
		}
		data.Set("urlbase", url)

		file.Prepare()

		for k, v := range file.Param() {
			data.Set(k, v)
		}

		context.Set("path.meta", file.Info())
		context.Set("path.tree", file.Tree())
		context.Set("path.content", "")

		log.Println("FileHandler", url, file.Type(), file.Name())

		buf := file.Content()

		log.Println("Handler: output length:", len(buf), ", type: ", file.Type())

		// Process templates
		//
		// Some types have predefined templates, some ARE templates. Predefined
		// templates are taken from the main context, while the content (this
		// path) is injected into the context so that the template can pick it up.

		switch file.Type() {
		case "revs":

			// Get the template used for revision lists
			tplx := context.Get("template.revs").String()

			// The data is in file.Tree()
			context.Set("path.data", file.Tree())

			name := filepath.Base(file.Name())
			if name[len(name)-1] == '@' {
				name = name[:len(name)-1]
			}
			context.Set("path.filename", name)

			if tplx != "" {
				// TODO preprocess templates !!
				tpl := ogdl.NewTemplate(tplx)
				buf = tpl.Process(context)
			} else {
				err = errors.New("Template not fount for type " + file.Type())
			}

		case "t":
			buf = file.Tree().Process(context)
		case "m":
			buf = file.Tree().Process(context)

			context.Set("path.content", string(buf))

			tplx := ""
			if strings.HasPrefix(strings.ToLower(filepath.Base(file.Name())), "readme.") {
				tplx = context.Get("template.mddir").String()
			} else {
				tplx = context.Get("template.md").String()
			}

			// log.Println("Handler: md: ", string(buf))

			if tplx != "" {
				// TODO preprocess templates !!
				tpl := ogdl.NewTemplate(tplx)
				buf = tpl.Process(context)
			} else {
				err = errors.New("Template not fount for type " + file.Type())
			}

		case "nb":
			context.Set("path.content", file.Content())

			tplx := ""
			if strings.HasPrefix(strings.ToLower(filepath.Base(file.Name())), "readme.") {
				tplx = context.Get("template.nb").String()
			} else {
				tplx = context.Get("template.nb").String()
			}

			if tplx != "" {
				// TODO preprocess templates !!
				tpl := ogdl.NewTemplate(tplx)
				buf = tpl.Process(context)
			} else {
				err = errors.New("Template not fount for type " + file.Type())
			}

		case "dir", "data/ogdl":

			// does the tree contain a template spec?
			tpln := file.Tree().Get("template").String()
			tplx := ""
			if tpln != "" {
				tplx = context.Get("template." + tpln).String()
			} else {
				if file.Type() == "dir" {
					tplx = context.Get("template.dir").String()
				} else {
					tplx = context.Get("template.data").String()
				}
			}
			if tplx != "" {
				// TODO preprocess templates !!
				tpl := ogdl.NewTemplate(tplx)
				buf = tpl.Process(context)

				//log.Println(" - template", tplx, file.Tree.Text())
			} else {
				err = errors.New("Template not fount for type " + file.Type())
			}
		}

		// Set Content-Type (MIME type)
		// <!doctype html> makes the browser picky about mime types. This is stupid.
		if len(file.Mime()) > 0 {
			w.Header().Set("Content-Type", file.Mime())
		}

		// w.Header().Set("Content-Disposition", "inline; filename=\"b.pdf\"")

		// Content-Length is set automatically in the Go http lib.

		if len(buf) == 0 {
			if file.Tree() != nil {
				w.Write([]byte(file.Tree().Text()))
			} else {
				if err == nil {
					err = ErrZeroLength
				}
				log.Println(err.Error())
				http.Error(w, err.Error(), 500)
			}
		} else {
			log.Println("Handler: writing to output", len(buf))
			w.Write(buf)
		}

	}

	return http.HandlerFunc(fn)
}
