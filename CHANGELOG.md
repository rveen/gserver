# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

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
