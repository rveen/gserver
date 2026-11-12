// Package ctxreg is a registry for template-context objects.
//
// Each external integration registers a named factory from its own init(),
// and the server installs every registered factory into its template contexts
// (gserver.Open, and again whenever .conf/context.ogdl is reloaded). This
// keeps the gserver package free of any import of the integrations
// themselves: a build adds one by blank importing its adapter package in its
// main package, nothing else.
//
// A factory receives the server configuration (.conf/config.ogdl), so that an
// integration can be built from its own section of it. It may be nil.
package ctxreg

import "github.com/rveen/ogdl"

// Factory produces the value installed in the template context under the
// name it was registered with. cfg is the server configuration, possibly nil.
type Factory func(cfg *ogdl.Graph) any

// providers maps a context name (the key used in templates) to its factory.
var providers = map[string]Factory{}

// Register records a factory under name. It is meant to be called from the
// init() of an adapter package. A later Register with the same name overrides
// the earlier one.
func Register(name string, factory Factory) {
	providers[name] = factory
}

// All returns the registered factories. The returned map is the live registry;
// callers must not mutate it.
func All() map[string]Factory { return providers }
