// Package filesctx registers golib/files under the "files" context name.
// Blank import it in main.go to enable: import _ ".../context/plugins/filesctx"
package filesctx

import (
	"github.com/rveen/golib/files"
	"github.com/rveen/gserver/v3/context/ctxreg"
	"github.com/rveen/ogdl/v2"
)

func init() {
	ctxreg.Register("files", func(*ogdl.Node) any { return &files.Files{} })
}
