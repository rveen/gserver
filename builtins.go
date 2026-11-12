package gserver

import (
	"log"
	"strings"

	"github.com/rveen/golib/document"
	"github.com/rveen/gserver/v2/context/ctxreg"
	"github.com/rveen/ogdl"
	rpc "github.com/rveen/ogdl/ogdlrf"
)

// InstallContext installs into the server's template contexts (the global one
// and, with multiple hosts, each host's):
//   - the built-in document and template functions (T, DOC, DOCV, docPart, …);
//   - the remote functions of the config's `ogdlrf` section;
//   - every plugin registered with ctxreg, built from the server config.
//
// Open calls it, and WatchContext calls it again on each reload. A build that
// sets up its server without Open calls it once before serving.
func (srv *Server) InstallContext() {
	srv.ContextMu.Lock()
	defer srv.ContextMu.Unlock()
	srv.installInto(srv.Context)
	for _, g := range srv.HostContexts {
		srv.installInto(g)
	}
}

// installInto installs the context values into g, which may be nil.
func (srv *Server) installInto(g *ogdl.Graph) {
	if g == nil {
		return
	}
	for name, v := range srv.builtins() {
		g.Set(name, v)
	}
	srv.setRemoteFunctions(g)
	for name, factory := range ctxreg.All() {
		g.Set(name, factory(srv.Config))
	}
}

// registerRemoteFunctions installs the `ogdlrf` remote functions into every
// context the server has.
func (srv *Server) registerRemoteFunctions() {
	srv.setRemoteFunctions(srv.Context)
	for _, g := range srv.HostContexts {
		srv.setRemoteFunctions(g)
	}
}

// setRemoteFunctions installs a client for each entry of the config's
// `ogdlrf` section into g:
//
//	ogdlrf
//	  name
//	    host localhost:1135
//	    protocol 2
func (srv *Server) setRemoteFunctions(g *ogdl.Graph) {
	if g == nil || srv.Config == nil {
		return
	}
	rfs := srv.Config.Get("ogdlrf")
	if rfs == nil {
		return
	}
	for _, rf := range rfs.Out {
		name := rf.ThisString()
		host := rf.Get("host").String()
		proto := rf.Get("protocol").Int64(2)
		log.Println("remote function registered:", name, host, proto)
		f := rpc.Client{Host: host, Timeout: 1, Protocol: int(proto)}
		g.Set(name, f.Call)
	}
}

// builtins returns the built-in template functions.
//
// The functions ending in V render with the request context, so that a
// document can use template variables, and pass the result through
// srv.DocFilter for a logged-in user.
func (srv *Server) builtins() map[string]any {

	filter := func(context *ogdl.Graph, s string) string {
		if srv.DocFilter == nil {
			return s
		}
		if user := context.Get("user").String(); user != "" {
			s = srv.DocFilter(s, user)
		}
		return s
	}

	return map[string]any{
		"T":                template,
		"DOC":              doc,
		"DocLinks":         docLinks,
		"DocLinksNumbered": docLinksNumbered,
		"docData":          docData,
		"docDataPath":      docDataPath,
		"docDataHtml":      docDataHtml,
		"docPart":          docPart,
		"docPartNoHeader":  docPartNoHeader,
		"docPartP1":        docPartP1,

		"DOCV": func(context *ogdl.Graph, doc string) []byte {
			d, _ := document.New(doc)
			d.Context = context
			return []byte(filter(context, d.Html()))
		},
		"DocLinksV": func(context *ogdl.Graph, doc, urlbase string) []byte {
			d, _ := document.New(doc)
			d.Context = context
			return []byte(filter(context, d.HtmlWithLinks(urlbase)))
		},
		"docPartV": func(context *ogdl.Graph, doc, path string) []byte {
			path = strings.ReplaceAll(path, "/", ".")
			d, _ := document.New(doc)
			d.Context = context
			d = d.Part(path)
			return []byte(filter(context, d.Html()))
		},
	}
}

func template(context *ogdl.Graph, template string) []byte {
	t := ogdl.NewTemplate(template)
	return t.Process(context)
}

func doc(doc string) []byte {
	d, _ := document.New(doc)
	return []byte(d.Html())
}

func docLinks(doc, urlbase string) []byte {
	d, _ := document.New(doc)
	return []byte(d.HtmlWithLinks(urlbase))
}

func docLinksNumbered(doc, urlbase string) []byte {
	d, _ := document.New(".nh\n" + doc)
	return []byte(d.HtmlWithLinks(urlbase))
}

func docData(doc string) *ogdl.Graph {
	d, _ := document.New(doc)
	return d.Data()
}

// docDataPath returns the part of a document's data at path ('/' or '.'
// separated).
func docDataPath(doc, path string) *ogdl.Graph {
	d, _ := document.New(doc)
	path = strings.ReplaceAll(path, "/", ".")
	return d.Data().Get(path)
}

// docDataHtml renders the part of a document's data at path as HTML, with
// links relative to base.
func docDataHtml(doc, path, base string) string {
	d, _ := document.New(doc)
	g := d.Data()
	path = strings.ReplaceAll(path, "/", ".")
	if path != "" {
		g = g.Get(path)
	}
	return g.Html(base)
}

func docPart(doc, path string) []byte {
	path = strings.ReplaceAll(path, "/", ".")
	d, _ := document.New(doc)
	d = d.Part(path)
	return []byte(d.Html())
}

func docPartNoHeader(doc, path string) []byte {
	d, _ := document.New(doc)
	d = d.Part(path)
	return []byte(d.HtmlNoHeader())
}

func docPartP1(doc, path string) string {
	d, _ := document.New(doc)
	d = d.Part(path)
	return d.Para1()
}
