// Package isolationctx registers electronics/isolation under the "isolation"
// context name.
// Blank import it in main.go to enable: import _ ".../context/plugins/isolationctx"
package isolationctx

import (
	"github.com/rveen/electronics/isolation"
	"github.com/rveen/gserver/v3/context/ctxreg"
	"github.com/rveen/ogdl/v2"
)

func init() {
	ctxreg.Register("isolation", func(*ogdl.Node) any { return &isolation.Isolation{} })
}
