// Package gosqlctx registers golib/gosql under the "sql" context name.
// Blank import it in main.go to enable: import _ ".../context/plugins/gosqlctx"
package gosqlctx

import (
	"github.com/rveen/golib/gosql"
	"github.com/rveen/gserver/context/ctxreg"
)

func init() {
	ctxreg.Register("sql", func() any { return &gosql.Db{} })
}
