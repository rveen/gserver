// Package htmlctx registers golib/html under the "html" context name.
// Blank import it in main.go to enable: import _ ".../context/plugins/htmlctx"
package htmlctx

import (
	"github.com/rveen/golib/html"
	"github.com/rveen/gserver/v3/context/ctxreg"
	"github.com/rveen/ogdl/v2"
)

func init() {
	ctxreg.Register("html", func(*ogdl.Node) any { return &html.Html{} })
}
