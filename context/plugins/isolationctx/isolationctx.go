// Package isolationctx registers electronics/isolation under the "isolation"
// context name.
// Blank import it in main.go to enable: import _ ".../context/plugins/isolationctx"
package isolationctx

import (
	"github.com/rveen/electronics/isolation"
	"github.com/rveen/gserver/context/ctxreg"
)

func init() {
	ctxreg.Register("isolation", func() any { return &isolation.Isolation{} })
}
