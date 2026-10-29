# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Fixed

- **A missing file is no longer answered with the directory's `index.*` under a
  200.** `GET /app/missing.js` returned `app/index.html` with `Content-Type:
  text/html` whenever the containing directory had an `index.*` or `readme.*`.
  Browsers only noticed for module scripts, where strict MIME checking rejects
  it; a stylesheet, image or source map at a stale path was silently accepted as
  a 200 carrying HTML, and every 404 in the application was invisible to logs and
  monitoring. The index fallback in `golib/fn/get.go` now applies only to path
  elements that are not naming a file — `docs/cap1`, `docs/1.2` and `docs/v0.98`
  continue into the index document as before. **Expect error rates to rise on
  first deploy: that is the fault becoming visible.**

- **A URL naming a directory now redirects to the same URL with a trailing
  slash** (301, GET and HEAD only, query preserved). Without it a browser given
  `/app` resolves every relative reference in the page against `/`, so
  `<script src="bundle.js">` requested `/bundle.js`. The probe uses the new
  literal `fn.FNode.IsDir`, so paths that merely resemble directories — `/doc`
  resolving to `doc.md`, `/doc/cap1`, a `_user` wildcard — are left alone. It
  runs after the auth check, so a 301 never reveals that a protected directory
  exists, and skips POST, which a client may reissue as a GET.

- **A directory with no `index.*` is served as a directory listing instead of
  `500 "Empty content"`.** Rendering a directory required a `dir` template in
  `.conf/config.ogdl`; without one the response was a 500 that blamed the
  request for a configuration gap. `gserver` now falls back to a built-in HTML
  listing — **a configured `dir` template still wins, so sites that have one are
  unaffected**. Entries beginning with `_` (path variables) are omitted and all
  names are HTML-escaped.

- **A zero-byte file is served as a 200 with an empty body.** The `len(content)
  == 0` test in `dynhandler.go` treated an empty `.txt` or placeholder `.js` as a
  server error. `Request.Process` now reports a genuine failure as an error, and
  a missing `document`/`data` template logs which template is missing rather than
  the misleading `Empty content`.

- **A directory holding several `index.*` files serves the right one.**
  `fn.index()` took the first match in readdir order, so a directory with both
  `index.css` and `index.html` answered the directory URL with the stylesheet.
  Candidates are now ranked: `index.*` before `readme.*`, then `.html`, `.htm`,
  `.md`, `.ogdl`, `.txt`, then anything else. A directory with a single candidate
  resolves exactly as before.

### Changed

- **The `-Fn` handler variants are now wrappers around a single implementation.**
  `statichandler-fn.go` and `dynhandler-fn.go` were copies of their base handlers
  that differed only in serving from a caller-supplied `*fn.FNode` rather than
  `srv.Root`, and the copies had drifted — `DynamicHandler` logged itself as
  `DynHandlerFn`. Both files are gone; `statichandler.go` and `dynhandler.go` each
  hold one unexported core taking an `fs *fn.FNode`, where `nil` selects
  `srv.Root`. `StaticFileHandler`, `StaticFileHandlerFn`, `DynamicHandler` and
  `DynamicHandlerFn` keep their signatures and behaviour, so callers such as
  `github.com/trukeio/gserver` are unaffected.

  A supplied `fs` continues to mean *trusted tree*: neither the `checkPath`
  login redirect nor the `httphook` interceptors run for it, and a miss falls
  back to `srv.Root`. That asymmetry predates this change and is now stated in
  the doc comments rather than left implicit in a duplicated file.

- **Static files are served with a uniform `Cache-Control: public, max-age=7200`.**
  `StaticFileHandlerFn` previously sent `max-age=36000` for embedded assets. The
  only effect is more frequent revalidation of embedded content.

- `StaticFileHandlerFn` and `DynamicHandlerFn` are deprecated. They remain for
  compatibility; new code should call the base handlers.

- **Startup and listener errors exit with status 1.** A missing
  `.conf/config.ogdl`, HTTPS without `-email`, or a port already in use used to
  log a line and exit 0, so a supervisor saw a clean stop and
  `Restart=on-failure` never fired. Errors are now written to stderr (even with
  `-log=false`) and the process exits 1. `Server.Serve` returns its error, `nil`
  after a regular `Shutdown()`; callers that ignore the result still compile.

- **Request log lines show the real client behind a local reverse proxy.** For
  requests from a loopback address carrying `X-Forwarded-For`, the address the
  proxy appended (the header's last entry) is logged instead of `127.0.0.1`. The
  header is ignored from any other peer, so remote clients cannot fake it. Only
  logging is affected; no access decision uses the client address.

### Added

- **`-root <dir>`**: change to the site directory at startup. All server paths
  (`.conf/*`, uploads, documents) are relative, so this replaces depending on
  the working directory.

- **Server options in `.conf/config.ogdl`.** Every flag except `-root` can be
  set in a `server` section (`host`, `secure_host`, `email`, `certs`,
  `timeout`, `session_timeout`, `max_sessions`, `userdb`, `multi`, `log`,
  `verbose`). Flags given on the command line win. An unknown key or invalid
  value stops startup; **an existing `server` section with other keys will now
  be rejected.**

- **Graceful shutdown on SIGTERM and SIGINT.** `srv.Shutdown()`, which existed
  but was never called, now runs on `systemctl stop` or Ctrl-C, so running
  requests finish; the process then exits 0.

- **`-certs <dir>`**: where Let's Encrypt certificates are stored (new
  `Server.CertDir`). The default, under `$HOME`, is fragile for a service user.

- Log lines carry no timestamp when running under journald (`JOURNAL_STREAM`
  set), which adds its own.

- `gserver/systemd/gserver@.service`: a template unit, one instance per site
  under `/srv/gserver/`, running as an unprivileged user with
  `CAP_NET_BIND_SERVICE`. The README documents installation, the config
  options and running behind Apache.

- Tests for the client address taken from `X-Forwarded-For`.

- Tests covering both handler branches: serving from an embedded `fs`, fallback
  to `srv.Root` on a miss, `404` when neither resolves, `checkPath` redirecting an
  anonymous request to `/login` when `fs` is `nil` and *not* redirecting when it
  is set, and `protect` returning `401`.

## [1.0.0] - 2026-07-10

The first tagged release. This file starts here: the project's earlier history
is untagged and is not documented below.

Session handling was reworked so that a server-side session exists only for
authenticated users. Some of the changes below are in the companion
`github.com/rveen/session2` package, released alongside this one as v1.1.0.

### Security

- **Closed an open redirect in the login handler.** `loginhandler.go` passed
  `r.FormValue("redirect")` straight to `http.Redirect`, so a link to
  `/login?redirect=https://evil.example` sent the user off-site immediately
  after a successful login. Redirect targets are now confined to a local path
  by `safeRedirect()`; anything else resolves to `/`.

- **Anonymous clients can no longer fill the session table.** Every request
  without a `sessid` cookie used to create and store a session. A client that
  ignores `Set-Cookie` — a crawler, or an attacker — created one session per
  request and could reach the 100 000 cap in about 100 seconds, after which
  every *new* visitor received `429 Number of open sessions exceeded` until the
  entries expired 30 minutes later. Existing cookie holders were unaffected, so
  the failure was easy to miss. Sessions are now created lazily, on the first
  request carrying an authenticated identity, and no unauthenticated code path
  writes to the table.

### Fixed

- **`WatchContext` no longer reinitialises the session manager.** Every write to
  `.conf/context.ogdl` called `srv.InitSessions()`, which discarded all stored
  sessions and reset `SessionTimeout` to its 30-minute default, silently
  undoing the `-ts` flag. Users were not logged out — identity lives in the
  signed `userid` cookie — but the `userACL` cache and any pending redirect
  were dropped, and the configured timeout was lost. A context reload now swaps
  only the context.

- **The session cap is enforced atomically.** `session2.Len() > srv.MaxSessions`
  followed by a separate `session2.Add()` was a check-then-act race, and the
  `>` comparison admitted `MaxSessions + 1` entries. The capacity check and the
  insert now happen under a single write lock inside `session2`.

- **`session2`'s global manager is swapped safely.** `Init()` reassigned a plain
  package variable that `Get`/`Add`/`Remove` read without synchronisation. It is
  now an `atomic.Pointer[manager]`.

- **The session map releases memory after a traffic spike.** Go maps never
  shrink their bucket array on delete, so a burst that filled the table raised
  the process floor permanently (~3.5 MB at 100 000 sessions, ~56 MB at one
  million). The cleaner now rebuilds the map once occupancy falls well below its
  high-water mark.

- **An unknown `Host` in multihost mode returns 500 instead of panicking.**
  `srv.HostContexts[r.Host]` could miss, leaving a nil parent context that
  `SessionContext` would dereference.

### Changed

- **At the session cap, the least recently used session is evicted** rather than
  new sessions being refused. A full table no longer locks out new logins; the
  displaced user re-authenticates transparently from their `userid` cookie.
  Eviction removes a small batch per scan so the cost is amortised across
  inserts. `Add` cannot fail, so the `429` response is gone — the surviving
  `nil` guard in `DynamicHandler`/`DynamicHandlerFn` now reports an unresolvable
  host context as `500`.

- **`MaxSessions` now bounds concurrent logins**, not anonymous traffic. The
  default of 100 000 is unchanged; measured cost is roughly 550 bytes per
  authenticated session, so the cap corresponds to about 55 MB.

- **The post-login redirect is stashed in a signed cookie** (`redirect`,
  `HttpOnly`, 600 s) instead of a session attribute. `/login` is reached
  anonymously, so a server-side stash would have reopened the path that lets an
  unauthenticated client allocate storage.

- `Request.Session` may now be `nil`, which is the normal case for an anonymous
  request.

### Added

- `-ms` flag on `gserver` and `gserver0` sets the maximum number of concurrently
  stored sessions. `0` leaves the default in place.
- `Server.SetMaxSessions(n)` adjusts the cap after `New()`, so flags applied
  after server construction take effect.
- `session2.Options.MaxSessions` and `session2.SetMaxSessions(n)`.
- Tests covering anonymous requests allocating no session, a 1000-request
  cookie-less flood leaving the table empty, lazy creation on authentication,
  redirect-stash round-tripping, open-redirect rejection, LRU eviction at the
  cap, map compaction, and the `Init` swap under `-race`.
