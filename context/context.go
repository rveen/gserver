package context

import (
	"github.com/rveen/electronics/isolation"
	"github.com/rveen/golib/document"
	"github.com/rveen/golib/files"
	"github.com/rveen/golib/gosql"
	"github.com/rveen/golib/html"
	str "github.com/rveen/golib/strings"
	"github.com/rveen/gserver"
	"github.com/rveen/ogdl"

	"log"
	"strings"
)

type ContextService struct{}

// Load the context for template processing
func (c ContextService) GlobalContext(srv *gserver.Server) {
	srv.Context.Set("T", template)
	srv.Context.Set("DOC", doc)
	srv.Context.Set("DocLinks", docLinks)
	srv.Context.Set("DocLinksNumbered", docLinksNumbered)
	srv.Context.Set("docData", docData)
	srv.Context.Set("docPart", docPart)
	srv.Context.Set("docPartNoHeader", docPartNoHeader)
	srv.Context.Set("docPartP1", docPartP1)
	srv.Context.Set("files", &files.Files{})
	srv.Context.Set("html", &html.Html{})
	srv.Context.Set("sql", &gosql.Db{})
	srv.Context.Set("strings", &str.Strings{})
	srv.Context.Set("isolation", &isolation.Isolation{})

	for _, c := range srv.HostContexts {
		c.Set("T", template)
		c.Set("DOC", doc)
		c.Set("docData", docData)
		c.Set("docPart", docPart)
		c.Set("DocLinks", docLinks)
		c.Set("DocLinksNumbered", docLinksNumbered)
		c.Set("docPartNoHeader", docPartNoHeader)
		c.Set("docPartP1", docPartP1)
		c.Set("files", &files.Files{})
		c.Set("html", &html.Html{})
		c.Set("sql", &gosql.Db{})
		c.Set("strings", &str.Strings{})
		c.Set("isolation", &isolation.Isolation{})
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
