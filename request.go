package gserver

import (
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/icza/session"
	"github.com/rveen/golib/fn"
	"github.com/rveen/ogdl"
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

func ConvertRequest(r *http.Request, w http.ResponseWriter, host bool, srv *Server) *Request {

	rq := &Request{HttpRequest: r}

	rq.Context = getSession(r, w, host, srv)
	rq.User = rq.Context.Get("user").String()

	// Add host name in case of multihost
	if host {
		rq.Path = r.Host + "/" + r.URL.Path
	} else {
		rq.Path = r.URL.Path
	}

	// load a pointer to a copy: do not touch srv.Root.
	// Achtung!!: this does not work: rq.File = &(*srv.Root)
	f := *srv.Root
	rq.File = &f

	return rq
}

func getSession(r *http.Request, w http.ResponseWriter, host bool, srv *Server) *ogdl.Graph {

	var context *ogdl.Graph

	sess := srv.Sessions.Get(r)

	// Get the context from the session, or create a new one
	if sess == nil {
		sess = session.NewSession()
		srv.Sessions.Add(sess, w)

		context = ogdl.New(nil)
		if !host {
			context.Copy(srv.Context)
		} else {
			context.Copy(srv.HostContexts[r.Host])
		}
		sess.SetAttr("context", context)
		context.Set("user", "nobody")

	} else {
		context = sess.Attr("context").(*ogdl.Graph)
	}

	// Add request specific parameters

	data := context.Create("R")
	data.Set("url", r.URL.Path)
	data.Set("home", srv.Root.Base)

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

	/*
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
		}*/

	return context
}

// Get is a direct map from URL to file (binary content + params)
func (r *Request) Get() error {
	// Get the file

	var err error

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
	if r.File.Type != "dir" {
		base = filepath.Dir(r.File.Path[len(r.File.Base):])
	}
	if base[len(base)-1] != '/' {
		base += "/"
	}

	data := r.Context.Node("R")
	data.Set("urlbase", base)

	// Add parameters found in the file path (_token)
	for k, v := range r.File.Params {
		data.Set(k, v)
	}
	return nil
}

// Process processes templates, remaining path fragments and sets mime type
//
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

		case "document":
			r.File.Content = []byte(r.File.Document.Html())
			fallthrough
		case "dir", "data", "log":
			log.Println("r.Process: ", r.File.Type)

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
				log.Println("no template for this type")
			}
			r.File.Content = tpl.Process(r.Context)
			r.Mime = "text/html"

		default: // 'file'. Check .text and .htm (templates)
			if strings.HasSuffix(r.File.Path, ".htm") || strings.HasSuffix(r.File.Path, ".text") {
				tpl := ogdl.NewTemplate(string(r.File.Content))

				r.Context.Set("mime", "")
				r.File.Content = tpl.Process(r.Context)

				// Allow templates to return arbitrary mime types
				mime := r.Context.Get("mime").String()
				if mime != "" {
					r.Mime = mime
					r.File.Content = r.Context.Get("content").Binary()
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