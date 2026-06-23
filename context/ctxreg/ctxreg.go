// Package ctxreg is a dependency-free registry for template-context objects.
//
// Each external integration registers a named factory from its own init(),
// and ContextService.GlobalContext installs every registered factory into the
// server contexts. This keeps context.go (and the gserver package) free of any
// import of the integrations themselves: a new dependency is added by blank
// importing its adapter package in main.go, nothing else.
package ctxreg

// providers maps a context name (the key used in templates) to a factory that
// produces a fresh value for that name.
var providers = map[string]func() any{}

// Register records a factory under name. It is meant to be called from the
// init() of an adapter package. A later Register with the same name overrides
// the earlier one.
func Register(name string, factory func() any) {
	providers[name] = factory
}

// All returns the registered factories. The returned map is the live registry;
// callers must not mutate it.
func All() map[string]func() any { return providers }
