package gserver

import (
	"golib/files"
	"log"
	"reflect"

	"github.com/microcosm-cc/bluemonday"
	"github.com/miekg/mmark"
	"github.com/russross/blackfriday"
	"github.com/rveen/ogdl"
)

// Load the context for template processing
//
func LoadContext(context *ogdl.Graph, srv *Server) {
	context.Set("T", template)
	context.Set("MD", xmarkdown)
	context.Set("files", &files.Files{})
	InitPlugins(context, srv)
}

func template(context *ogdl.Graph, template string) []byte {
	t := ogdl.NewTemplate(template)
	return t.Process(context)
}

func markdown(s string) []byte {
	u := blackfriday.MarkdownCommon([]byte(s))
	return bluemonday.UGCPolicy().SanitizeBytes(u)
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

func InitPlugins(context *ogdl.Graph, srv *Server) {

	for _, plugin := range srv.Plugins {

		fn := context.Node(plugin).Interface()

		// v := reflect.TypeOf(fn)
		v := reflect.ValueOf(fn)

		// m, isValid := v.MethodByName("Init")
		m := v.MethodByName("Init")
		if m.IsValid() {
			// if isValid {
			log.Println(" - Init method found")
			cfg := srv.Config.Get(plugin)

			if cfg != nil && cfg.Len() != 0 {

				var vargs []reflect.Value
				// vargs = append(vargs, reflect.ValueOf(fn))
				vargs = append(vargs, reflect.ValueOf(cfg))
				log.Printf(" - Init method to be called with: %s\n", cfg.Text())
				// m.Func.Call(vargs)
				m.Call(vargs)
			}
		}

		m = v.MethodByName("Host")
		if m.IsValid() {
			var args []reflect.Value
			args = m.Call(args)
			log.Printf("context.go, calling Host: %v\n", args)
		}

		context.Set(plugin, fn)
	}
}
