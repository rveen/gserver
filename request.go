package gserver

import (
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/chmike/securecookie"
	"github.com/rveen/golib/fn"
	"github.com/rveen/ogdl"
	"github.com/rveen/session"
)

type Request struct {
	HttpRequest *http.Request
	User        string
	Context     *ogdl.Graph
	Params      *ogdl.Graph
	Path        string
	File        *fn.FNode
	Mime        string
	// Session
}

var TplExtensions []string = []string{".htm", ".txt", ".csv", ".json", ".g", ".ogdl", ".xml", ".xlsx", ".svg"}

func ConvertRequest(r *http.Request, w http.ResponseWriter, host bool, srv *Server) *Request {

	rq := &Request{HttpRequest: r}

	rq.Context = getSession(r, w, host, srv)
	if rq.Context == nil {
		// Could not create a new session
		return nil
	}
	rq.User = rq.Context.Get("user").String()

	// Add host name in case of multihost
	if host {
		rq.Path = r.Host + "/" + r.URL.Path
	} else {
		rq.Path = r.URL.Path
	}

	// load a pointer to a copy: do not touch srv.Root.
	// Achtung!!: this doesn't work: rq.File = &(*srv.Root)
	f := *srv.Root
	rq.File = &f

	return rq
}

func getSession(r *http.Request, w http.ResponseWriter, host bool, srv *Server) *ogdl.Graph {

	var context *ogdl.Graph

	sess := srv.Sessions.Get(r)

	// Get the context from the session, or create a new one
	if sess == nil {

		if srv.Sessions.Len() > srv.MaxSessions {
			log.Println("max number of session reached:", srv.MaxSessions)
			return nil
		}

		sess := session.NewSessionOptions(&session.SessOptions{Timeout: 30 * time.Minute})
		// sess = session.NewSession()
		log.Println("session created. Total number:", srv.Sessions.Len())
		srv.Sessions.Add(sess, w)

		context = ogdl.New(nil)
		if !host {
			context.Copy(srv.Context)
		} else {
			context.Copy(srv.HostContexts[r.Host])
		}
		sess.SetAttr("context", context)

	} else {
		context = sess.Attr("context").(*ogdl.Graph)
	}

	// Iff the userCookie is set, the set 'user' to its value
	user := UserCookieValue(r)
	if user != "" && user != "-" {
		context.Set("user", user)
	}

	// If there is no user set and there is an auto-login user defined:
	u := context.Node("user").String()
	if u == "" || u == "nobody" && srv.DefaultUser != "" {
		context.Set("user", srv.DefaultUser)
	}

	log.Printf("getSession: user is %s\n", context.Node("user").String())

	// Add request specific parameters

	data := context.Create("R")
	ur := r.URL.Path
	if u == "" {
		ur = "/"
	}
	data.Set("url", ur)
	data.Set("home", srv.Root.Root)

	r.ParseForm()

	// Add GET, POST, PUT parameters into context
	for k, vv := range r.Form {

		var n *ogdl.Graph

		for _, v := range vv {
			if strings.HasSuffix(k, "._ogdl") {
				k = k[0 : len(k)-6]
				g := ogdl.FromString(v)
				if n == nil {
					data.Set(k, g)
					n = data.Get(k)
				} else {
					n.Add(g)
				}
			} else {
				if n == nil {
					data.Set(k, v)
					n = data.Get(k)
				} else {
					n.Add(v)
				}
			}
		}
	}

	return context
}

// Get is a direct map from URL to file (binary content + params)
// TODO: Clean r.Path vs r.File.Path dicotomy
func (r *Request) Get() error {
	// Get the file

	var err error

	// log.Printf("URL.Path (0): %s\n", r.Path)

	if r.HttpRequest.FormValue("m") == "raw" {
		err = r.File.GetRaw(r.Path)
	} else {
		err = r.File.Get(r.Path)
	}

	if err != nil {
		return err
	}

	// Set R.urlbase (for setting <base href="$R.urlbase"> allowing relative URLs)
	base := r.HttpRequest.URL.Path
	// log.Printf("URL.Path: %s\n", base)

	if r.File.Type != "dir" && strings.HasPrefix(r.File.Path, r.File.Root) {
		base = filepath.Dir(r.File.Path[len(r.File.Root):])
	}
	base = strings.ReplaceAll(base, "\\", "/")
	if len(base) > 1 && base[len(base)-1] != '/' {
		base += "/"
	}

	// log.Printf("URL.Path (3): %s\n", base)

	data := r.Context.Node("R")
	data.Set("urlbase", base)

	// Add parameters found in the file path (_token)
	for k, v := range r.File.Params {
		data.Set(k, v)
	}
	return nil
}

// Process processes templates, remaining path fragments and sets mime the type
func (r *Request) Process(srv *Server) error {
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

	r.Mime = ""

	if r.HttpRequest.FormValue("m") != "raw" {
		switch r.File.Type {

		case "document", "document_dir":
			r.File.Document.Context = r.Context
			r.File.Content = []byte(r.File.Document.Html())
			fallthrough
		case "dir", "data", "log":
			// log.Println("r.Process: ", r.File.Type)

			r.Context.Set("path.content", string(r.File.Content))
			r.Context.Set("path.data", r.File.Data)

			tp := r.HttpRequest.FormValue("t")
			if tp == "" {
				if strings.HasSuffix(r.File.Path, "readme.md") {
					tp = "readme"
				} else {
					tp = r.File.Type
				}
			}

			tpl := srv.Templates[tp]
			if tpl == nil {
				log.Println("no template for type", tp)
			}
			r.File.Content = tpl.Process(r.Context)
			r.Mime = "text/html"

		default: // 'file'. Check if it is a template
			if hasTplExtension(r.File.Path) {
				tpl := ogdl.NewTemplate(string(r.File.Content))

				r.Context.Set("mime", "")
				r.File.Content = tpl.Process(r.Context)

				// Allow templates to return arbitrary mime types
				mime := r.Context.Get("mime").String()
				if mime != "" {
					r.Mime = mime
					r.File.Content = r.Context.Get("content").Bytes()
				}
			}
		}
	} else {
		// raw content with template
		if r.HttpRequest.FormValue("t") != "" {
			tpl := srv.Templates[r.HttpRequest.FormValue("t")]
			r.File.Content = tpl.Process(r.Context)
			r.Mime = "text/html"
		}
	}

	// Set Content-Type (MIME type)
	if r.Mime == "" {
		ext := filepath.Ext(r.File.Path)
		r.Mime = mime.TypeByExtension(ext)
	}

	return nil
}

func hasTplExtension(s string) bool {
	for _, v := range TplExtensions {
		if strings.HasSuffix(s, v) {
			return true
		}
	}

	return false
}

func UserCookie() *securecookie.Obj {

	key := []byte("f8hk39o9mx0dmrn1pa39jfla39djm3f0")
	userCookie := securecookie.MustNew("userid", key, securecookie.Params{
		Path:   "/",
		MaxAge: 0,
		Secure: false, // cookie received with HTTP for testing purpose
	})
	return userCookie
}

func UserCookieValue(r *http.Request) string {

	userCookie := UserCookie()

	b, err := userCookie.GetValue(nil, r)
	if err != nil {
		return ""
	}
	return string(b)
}

func DeleteUserCookie(w http.ResponseWriter) {

	c := &http.Cookie{
		Name:     "userid",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	}

	http.SetCookie(w, c)
}
