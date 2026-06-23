package context

import (
	"github.com/rveen/golib/document"
	"github.com/rveen/gserver"
	"github.com/rveen/gserver/context/ctxreg"
	"github.com/rveen/ogdl"

	"log"
	"strings"
)

type ContextService struct{}

// Load the context for template processing.
//
// The built-in document/template helpers live here; external dependencies are
// pulled from ctxreg, where each integration self-registers from its adapter
// package (see context/plugins/*). Both the global context and every per-host
// context receive the same set, so there is a single source of truth.
func (c ContextService) GlobalContext(srv *gserver.Server) {

	builtins := map[string]any{
		"T":                template,
		"DOC":              doc,
		"DocLinks":         docLinks,
		"DocLinksNumbered": docLinksNumbered,
		"docData":          docData,
		"docPart":          docPart,
		"docPartNoHeader":  docPartNoHeader,
		"docPartP1":        docPartP1,
	}

	apply := func(g *ogdl.Graph) {
		for name, fn := range builtins {
			g.Set(name, fn)
		}
		for name, factory := range ctxreg.All() {
			g.Set(name, factory())
		}
	}

	apply(srv.Context)
	for _, c := range srv.HostContexts {
		apply(c)
	}
}

func template(context *ogdl.Graph, template string) []byte {
	t := ogdl.NewTemplate(template)
	return t.Process(context)
}

func doc(doc string) []byte {
	d, _ := document.New(doc)
	s := d.Html()
	return []byte(s)
}

func docLinks(doc, urlbase string) []byte {
	d, _ := document.New(doc)
	s := d.HtmlWithLinks(urlbase)
	return []byte(s)
}

func docLinksNumbered(doc, urlbase string) []byte {
	d, _ := document.New(".nh\n" + doc)
	s := d.HtmlWithLinks(urlbase)
	return []byte(s)
}

func docData(doc string) *ogdl.Graph {
	d, _ := document.New(doc)
	g := d.Data()

	return g
}

func docPart(doc, path string) []byte {

	log.Println("docPart", path)

	path = strings.ReplaceAll(path, "/", ".")

	d, _ := document.New(doc)
	d = d.Part(path)
	s := d.Html()
	return []byte(s)
}

func docPartNoHeader(doc, path string) []byte {

	log.Println("docPartNoHeader", path)

	d, _ := document.New(doc)
	log.Println("docPartNoHeader", d.Raw().Text())

	d = d.Part(path)
	s := d.HtmlNoHeader()
	return []byte(s)
}

func docPartP1(doc, path string) string {
	d, _ := document.New(doc)
	d = d.Part(path)
	return d.Para1()
}
