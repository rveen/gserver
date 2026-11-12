// Package htmlctx registers golib/html under the "html" context name.
// Blank import it in main.go to enable: import _ ".../context/plugins/htmlctx"
package htmlctx

import (
	"github.com/rveen/golib/html"
	"github.com/rveen/gserver/v2/context/ctxreg"
	"github.com/rveen/ogdl"
)

func init() {
	ctxreg.Register("html", func(*ogdl.Graph) any { return &html.Html{} })
}
