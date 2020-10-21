package context

import (
	"log"

	"github.com/rveen/golib/document"
	"github.com/rveen/golib/files"
	"github.com/rveen/gserver"
	"github.com/rveen/gserver/html"

	"github.com/miekg/mmark"
	"github.com/rveen/markdown"
	"github.com/rveen/markdown/parser"
	"github.com/rveen/ogdl"
)

type ContextService struct{}

// Load the context for template processing
//
func (c ContextService) Load(context *ogdl.Graph, srv *gserver.Server) {
	context.Set("T", template)
	context.Set("MD", xxmarkdown)
	context.Set("MDX", xmarkdown)
	context.Set("DOC", doc)
	context.Set("files", &files.Files{})
	context.Set("html", &html.Html{})
}

func template(context *ogdl.Graph, template string) []byte {
	t := ogdl.NewTemplate(template)
	return t.Process(context)
}

func doc(context *ogdl.Graph, doc string) []byte {
	d, _ := document.New(doc)
	s := document.ToHtml(d)
	return []byte(s)
}

const extensions int = mmark.EXTENSION_TABLES | mmark.EXTENSION_FENCED_CODE |
	mmark.EXTENSION_AUTOLINK | mmark.EXTENSION_SPACE_HEADERS |
	mmark.EXTENSION_CITATION | mmark.EXTENSION_TITLEBLOCK_TOML |
	mmark.EXTENSION_HEADER_IDS | mmark.EXTENSION_AUTO_HEADER_IDS |
	mmark.EXTENSION_UNIQUE_HEADER_IDS | mmark.EXTENSION_FOOTNOTES |
	mmark.EXTENSION_SHORT_REF | mmark.EXTENSION_INCLUDE | mmark.EXTENSION_PARTS |
	mmark.EXTENSION_ABBREVIATIONS | mmark.EXTENSION_DEFINITION_LISTS

// MDX processes extended markdown
func xmarkdown(s string) []byte {

	htmlFlags := 0
	renderer := mmark.HtmlRenderer(htmlFlags, "", "")
	return mmark.Parse([]byte(s), renderer, extensions).Bytes()
}

// MDX processes extended markdown

func xxmarkdown(s string) []byte {

	//extensions := parser.NoIntraEmphasis | parser.Tables | parser.FencedCode |
	//	parser.Autolink | parser.Strikethrough | parser.SpaceHeadings | parser.HeadingIDs |
	//	parser.BackslashLineBreak | parser.DefinitionLists
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs
	p := parser.NewWithExtensions(extensions)
	println("----------------> xxmarkdown()")
	return markdown.ToHTML([]byte(s), p, nil)
}

type DomainConfig struct{}

var tpl = ogdl.NewTemplate("$(u=conector.Search('id1=1 x0='+R.user+' y=h')) $(u=u.result.item)")

func (d DomainConfig) GetConfig(ctx *ogdl.Graph, domain string, level int) *ogdl.Graph {

	log.Println("GetConfig", domain, ctx.Get("R.user").String())

	u := ctx.Get("u")

	if u.Len() > 0 {
		d := u.Get("x0").String()
		if d == domain {
			return u
		}
	}

	tpl.Process(ctx)
	u = ctx.Get("u")
	log.Println("GetConfig: u loaded from conector")
	return u

}
