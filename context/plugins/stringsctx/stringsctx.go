// Package stringsctx registers golib/strings under the "strings" context name.
// Blank import it in main.go to enable: import _ ".../context/plugins/stringsctx"
package stringsctx

import (
	str "github.com/rveen/golib/strings"
	"github.com/rveen/gserver/v3/context/ctxreg"
	"github.com/rveen/ogdl/v2"
)

func init() {
	ctxreg.Register("strings", func(*ogdl.Node) any { return &str.Strings{} })
}
