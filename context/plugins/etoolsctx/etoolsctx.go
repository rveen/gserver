// Package etoolsctx registers electronics/etools under the "etools" context name.
// Blank import it in main.go to enable: import _ ".../context/plugins/etoolsctx"
package etoolsctx

import (
	"github.com/rveen/electronics/etools"
	"github.com/rveen/gserver/context/ctxreg"
)

func init() {
	ctxreg.Register("etools", func() any { return &etools.Etools{} })
}
