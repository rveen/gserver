# Session handling

## Overview

Sessions are managed by `github.com/rveen/session` using a secure cookie
(`userid`) to identify the client. The session store is in-memory with a
30-minute idle timeout. The maximum number of concurrent sessions is enforced
by `srv.MaxSessions`; requests that would exceed this limit receive a nil
context and are rejected.

The entry point is `getSession` in `request.go`, called once per HTTP request
from `ConvertRequest`. It returns a `*ogdl.Graph` that is stored in
`Request.Context` and used throughout the request lifecycle — by middleware,
handlers, and the OGDL template engine (`tpl.Process(r.Context)`).

## What a session stores

Sessions store only two plain string attributes:

| Attribute | Type   | Set when                              |
|-----------|--------|---------------------------------------|
| `"user"`  | string | A valid user cookie is present        |
| `"userACL"` | string | ACL is resolved from the DB for the first time (`GetACL`) |

No `*ogdl.Graph` is stored in the session. The server's global context
(`srv.Context`, populated from `.conf/context.ogdl` and Go function
registrations) is shared read-only across all sessions and requests.

## Per-request context construction

Each request builds its own `*ogdl.Graph` through `SessionContext`
(`sessionctx.go`). This avoids both session-level graph copies and
concurrent-write races.

### SessionContext

`SessionContext` holds:
- `local *ogdl.Graph` — a small graph with the request's own nodes
  (`user`, `userACL`, `R` and its children).
- `Parent *ogdl.Graph` — a pointer to `srv.Context` (or a host-specific
  context); never written to.

The methods `Set`, `Get`, `Node`, and `Create` operate on `local` directly.
`Get` and `Node` fall back to `Parent` when the key is not found locally.

### Graph() — the merged view for the template engine

The template engine (`tpl.Process`) requires a concrete `*ogdl.Graph`.
`SessionContext.Graph()` builds one per request:

1. Local nodes are added first via `AddNodes(sc.local)`. Because
   `ogdl.Graph.Node()` returns the **first** match in `Out`, these shadow
   any same-named entry in the parent.
2. For each top-level node `n` in `Parent.Out`, a **shell node** is appended:
   `&ogdl.Graph{This: n.This, Out: n.Out}`. The shell is a new heap object
   that shares the parent's child slice by value (a slice header copy). If the
   template engine calls `set` on the shell (e.g. `$(title=ritem.x1)`), it
   clears and replaces the shell's `Out` field without touching the parent's
   actual node.

Cost: ~35 shell allocations (~1.4 KB) + ~10 local nodes (~0.4 KB) per
request, versus a full recursive copy of ~75 nodes (~3 KB) in the previous
implementation.

### getSession flow

```
srv.Sessions.Get(r)
  └─ nil → create new session, Add to store
  └─ existing → restore "user" and "userACL" from session string attrs

newSessionContext(parent)          // parent = srv.Context or host context

Restore from session:
  sess.Attr("user")   → sc.Set("user", ...)
  sess.Attr("userACL") → sc.Set("userACL", ...)

UserCookieValue(r):
  if non-empty → sc.Set("user", ...), sess.SetAttr("user", ...)

srv.DefaultUser fallback (if user still empty)

GetACL(user, srv):
  if userACL not cached → resolve, sc.Set("userACL", ...), sess.SetAttr("userACL", ...)

sc.Create("R") → populate R.url, R.home, form params

return sc.Graph()   // merged *ogdl.Graph for template engine
```

## Concurrency safety

The original code stored a single `*ogdl.Graph` per session and returned the
same pointer to all concurrent requests. Because `ogdl.Graph` has no internal
mutex, concurrent `Set` calls raced on the `Out []*Graph` slice — one
goroutine's `append` could replace the backing array while another was
ranging over it, causing a nil-pointer dereference.

The current design eliminates this: each request builds its own
`SessionContext` with its own `local` graph. No graph object is shared between
concurrent requests. The only shared object is `srv.Context` (and host
contexts), which is protected by `srv.ContextMu` during the pointer read at
the start of `getSession` and is never written to after server startup
(except during a `WatchContext` reload, which holds `ContextMu.Lock()`).

## Memory impact

| Metric | Before | After |
|--------|--------|-------|
| Per-session heap | ~3 KB (`*Graph` copy of srv.Context) | ~50 bytes (2 strings) |
| 1 000 sessions | ~3 MB | ~50 KB |
| Per-request allocations | ~75 nodes (full deep copy) | ~45 nodes (10 local + 35 shells) |

## ACL caching

`GetACL` queries the database and can be slow. The result is cached in the
session as the plain string `"userACL"` and restored into `sc.local` on every
subsequent request. If the ACL changes in the database, it takes effect only
after the session expires (up to 30 minutes) or the server is restarted.
