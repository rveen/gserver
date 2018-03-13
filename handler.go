package gserver

import (
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	// fr "github.com/DATA-DOG/fastroute"
	"github.com/icza/session"
	"github.com/rveen/ogdl"
)

// FileHandler processes all paths that exist in the file system starting
// from the root directory, whether they are static files or templates or markdown.
//
// NOTE This handler is a final one. If the path doesn't exist, it returns 'Not found'
// NOTE This handler needs context information (access to Server{})
// NOTE See https://github.com/bpowers/seshcookie
//
func FileHandler(srv *Server) http.Handler {

	fn := func(w http.ResponseWriter, r *http.Request) {

		//log.Printf("FileHandler pattern %s\n----\n%v\n---\n", fr.Pattern(r), fr.Parameters(r))

		// Get a session, whether or not the user has logged in
		sess := srv.Sessions.Get(r)
		if sess == nil {
			sess = session.NewSession()
			sess.SetAttr("user", "nobody")
			srv.Sessions.Add(sess, w)
		} else if r.FormValue("Logout") != "" {
			log.Println("Logout requested")
			srv.Sessions.Remove(sess, w)
			sess = session.NewSession()
			sess.SetAttr("user", "nobody")
			srv.Sessions.Add(sess, w)
		}

		// Login if requested

		if r.FormValue("Login") != "" {

			// Here the code to access an identity service
			user, err := srv.Login.Auth(r, srv)

			log.Println("Login requested", user)

			if err == nil {
				sess.SetAttr("user", user)
				if rdir := r.FormValue("redirect"); rdir != "" {
					if rdir == "_user" {
						rdir = "/" + user
					}
					http.Redirect(w, r, rdir, 302)
					return
				}
			} else {
				http.Error(w, http.StatusText(401), 401)
				return
			}
		}

		// Upload files if "UploadFiles" is present

		// Login if requested

		if r.FormValue("UploadFiles") != "" {

			// Handle file uploads. We call ParseMultipartForm here so that r.Form[] is
			// initialized. If it isn't a multipart this gives an error that we are ignoring.
			err := r.ParseMultipartForm(10000000) // 10M

			for {
				if err != nil {
					break
				}

				//Where to store the file
				folder := r.FormValue("folder")
				if len(folder) == 0 {
					folder = "default"
				}
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

		// Get the file
		url := filepath.Clean(r.URL.Path)
		file, params, _ := srv.Root.Get(url, true)

		if file == nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		// log.Println("FileHandler", url, file.Type)

		buf := file.Content

		// If we serve a template, create a Context for it.
		// The base context comes from context.g
		//
		// GET, POST, FilePath parameters, and user have to be added

		if file.Type == "t" || file.Type == "m" {
			var context *ogdl.Graph
			i := sess.Attr("context")

			if i == nil {
				context = ogdl.New()
				context.Copy(srv.Context)
				sess.SetAttr("context", context)
				srv.ContextService.Load(context, srv)
			} else {
				context = i.(*ogdl.Graph)
			}
			context.Set("user", sess.Attr("user"))
			context.Substitute("$_user", sess.Attr("user"))

			data := context.Create("R")
			data.Set("url", r.URL.Path)

			r.ParseForm()

			// Add GET, POST, PUT parameters to context
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

			for k, v := range params {
				data.Set(k, v)
			}

			if file.Type == "t" {
				buf = file.Tree.Process(context)
			} else {
				buf = context.Get("mdheader").Process(context)
				buf2 := context.Get("mdfooter").Process(context)
				buf = append(buf, append(file.Content, buf2...)...)
			}
		}

		// Set Content-Type (MIME type)
		// <!doctype html> makes the browser picky about mime types. This is stupid.
		if len(file.Mime) > 0 {
			w.Header().Set("Content-Type", file.Mime)
		}

		// Content-Length is set automatically in the Go http lib.

		if len(buf) == 0 {
			log.Println("Zero length file")
			http.Error(w, "Zero length file", 500)
		} else {
			w.Write(buf)
		}
	}

	return http.HandlerFunc(fn)

}
