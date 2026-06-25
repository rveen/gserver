# EVENTS

Known issues and future work, recorded for later implementation if/when needed.

## Latent: shared-node mutation through the per-request SessionContext overlay

**Status:** latent — not reachable with the current `context.ogdl` files and templates.
Documented now so it is on record; implement the fix only if the trigger condition
below becomes reachable.

### Background

`SessionContext` (`sessionctx.go`) is a lightweight per-request overlay over the
read-only server context (`srv.Context` or a host context). It was introduced to
avoid deep-copying the whole context graph on every request. Only session-specific
values (`user`, `userACL`, `R.*`) are stored in `sc.local`; everything else resolves
from `Parent` without copying.

When the merged graph is built for the template engine, `Graph()` wraps each
parent top-level node in a fresh shell but **shares its child slice by reference**:

```go
// sessionctx.go
for _, n := range sc.Parent.Out {
    merged.Out = append(merged.Out, &ogdl.Graph{This: n.This, Out: n.Out}) // n.Out shared
}
```

So only the *top-level* parent nodes are isolated (new shell structs). Their
children (`n.Out`) are the very same slice/nodes held by the shared, supposedly
read-only `srv.Context`.

### The problem

A write reaches that shared memory through `ogdl.Graph.set` (`graph.go`) whenever a
path has **two or more segments and the first segment names an existing parent key**:

```go
node = node.Node(elem.ThisString()) // 1st segment -> the SHELL (Out shared with parent)
// ...descend into the shared slice via the 2nd segment...
node.Out = nil                      // wipes the shared node's children, OR
node = node.Add(elem.This)          // appends into the shared backing array
```

Consequences if triggered:
- One request mutates / clears children of a node that the global `srv.Context`
  and all other concurrent requests see.
- `Add` appending into a slice whose backing array is shared with the parent is a
  classic slice-aliasing data race across concurrent requests (`*ogdl.Graph` has no
  internal mutex).

Note: a **single-segment** write to a shared scalar (e.g. `theme="x"`) is safe —
`set` reassigns the shell struct's own `Out` field and never touches the parent
slice. The hazard requires `firstSegment.secondSegment = ...` where `firstSegment`
exists in the parent context.

### Why it is not currently reachable

- The shared keys in the active `context.ogdl` files (`title`, `theme`, `header`,
  `navbar`, `footer`, `sidebar`, `tabs_item`, ...) are all scalars or block-scalar
  template fragments — **none is a navigable sub-tree**, so no template path-write
  can descend into shared structure.
- Template writes observed in `../kf1/frontend` land on either `R.*` (created in
  `sc.local`, per-request) or freshly-created top-level vars (`items`, `ritem`,
  `editable`, `$for` loop vars via `c.Add`) — all owned per-request.
- The one framework site with the dangerous shape,
  `request.go:258-259`:

  ```go
  r.Context.Set("path.content", string(r.File.Content))
  r.Context.Set("path.data", r.File.Data)
  ```

  is safe only because no `context.ogdl` defines a top-level `path` key, so `path`
  is created fresh in the merged graph each request.

### Trigger condition to watch for

The bug becomes live if **either**:
1. a host's `context.ogdl` gains a top-level `path` node (then `request.go:258-259`
   would corrupt it), or
2. any template/handler performs a multi-segment assignment whose first segment is
   a pre-existing shared context key (e.g. `$(theme.x = ...)`).

### Proposed fix (future)

Copy-on-write a parent shell's `Out` before mutating any descendant. Options:
- In `ogdl.Graph.set`, when about to mutate a node that was reached by descending
  into a shared (shell) subtree, clone that subtree first, or
- In `SessionContext.Graph()` / `Set`, when a write targets a path under a
  parent-derived key, materialize a private copy of that branch into `sc.local`
  (shadowing the parent), so the merge step never exposes shared child slices to
  mutation.

Either approach preserves the no-deep-copy fast path for the common case (writes to
`R.*` and brand-new top-level vars) while making nested writes into parent keys
safe.

### References

- `sessionctx.go` — `SessionContext`, `Graph()`
- `request.go` — overlay construction (`getSession`), `path.content`/`path.data` writes
- `../ogdl/graph.go` — `Graph.set`, `Graph.Copy` (interface/pointer copy semantics)
