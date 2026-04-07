# gserver — Claude Code Context

## Project Overview

`gserver` is a Go HTTP server that serves both static and dynamic content. It uses `github.com/rveen/golib/fn` as its filesystem abstraction layer and `github.com/rveen/ogdl` for templating and data.

## Repository Layout

```
gserver/
  server.go            — Server struct, Serve(), HTTP config, TLS via certmagic
  filehandler.go       — FileHandler(): thin wrapper around http.FileServer (streaming, Range-safe)
  statichandler.go     — StaticFileHandler(): reads whole file into memory, w.Write(content)
  statichandler-fn.go  — StaticFileHandlerFn(): same as above, alternate entry
  dynhandler.go        — DynamicHandler(): full dynamic pipeline (session, auth, Get, Process, Write)
  dynhandler-fn.go     — DynamicHandlerFn(): same with fallback to srv.Root
  request.go           — Request struct, ConvertRequest(), Request.Get(), Request.Process()
  upload.go            — fileUpload(): multipart upload handler
  gserver/main.go      — Entry point: route registration
```

Key external packages (siblings in `../golib/`):
- `fn/` — filesystem abstraction: `FNode`, `Get()`, `file()` (`os.ReadFile`), SVN/Git backends
- `document/` — Markdown → HTML rendering via `strings.Builder`
- `fs/` — older filesystem layer (not the primary path)

## Request → Response Flow

### Route dispatch (`gserver/main.go`)
| Route | Handler | Notes |
|---|---|---|
| `/favicon.ico` | `StaticFileHandler` | whole-file in memory |
| `/files/*` | `FileHandler` (→ `http.FileServer`) | **streaming, Range-safe** |
| `/static/*`, `/file/*` | `StaticFileHandler` | whole-file in memory |
| `/*` | `LoginAdapter` → `DynamicHandler` | whole-file in memory |

### Static path
1. `StaticFileHandler` shallow-copies `*srv.Root` (`FNode`)
2. `file.Get(path)` → `fn.get()` → `fn.file()` → **`os.ReadFile()`** → `fn.Content []byte`
3. `w.Write(file.Content)` — single write of entire byte slice

### Dynamic path
1. `ConvertRequest()` — session management, form parsing
2. `r.Get()` → same `fn.file()` → `os.ReadFile()`
3. `r.Process(srv)` — may further transform `r.File.Content`:
   - `.md` → `Document.Html()` (strings.Builder) → `[]byte`
   - template extensions (`.htm`, `.html`, `.txt`, etc.) → `ogdl.NewTemplateFromBytes(content)` → `.Process()`
4. `w.Write(r.File.Content)` — single write

## Known Large-File Issues

> **Root cause**: every route except `/files/*` loads the entire file into a `[]byte` before writing a single byte to the client. There is no streaming, no `http.ServeContent`, and no `Range` request support.

| Issue | Location | Impact |
|---|---|---|
| `os.ReadFile` — no size limit | `fn/sys.go:76` | OOM for large files |
| `w.Write(full []byte)` — no streaming | `statichandler.go:62`, `dynhandler.go:78` | No Range support; memory ∝ file size |
| `exec.Command.Output()` for SVN `cat` | `fn/svn.go:156`, `fs/svnfs/fs.go:130` | OOM; stderr truncated >64 KB |
| Markdown triple-copy in memory | `document/document.go:58`, `request.go:220` | Peak ~3–5× file size per request |
| ~~Template `string([]byte)` copy~~ | `request.go:247` | **Fixed** — now uses `ogdl.NewTemplateFromBytes` |
| Write timeout on whole-content write | `server.go:261` (default 10 s) | Truncated response for slow clients |

There is a `TODO` comment in `dynhandler.go:16`:
```go
// TODO serve files with http.ServeContent (handles large files with Range requests)
```

## Fix Direction

To fix large-file serving without breaking dynamic content:

1. **Binary/opaque files** served statically: open the file, pass `*os.File` to `http.ServeContent(w, r, name, modtime, f)` — this handles Range, ETag, Content-Length, streaming.
2. **Dynamic/template files**: these must stay buffered (template output is not seekable), but add a size guard before calling `os.ReadFile` to reject files above a threshold.
3. **SVN backend**: replace `exec.Command.Output()` with `cmd.StdoutPipe()` + `io.Copy` to avoid buffering large blobs in memory.

## Build & Run

```bash
cd gserver
go build ./...
go run ./gserver/main.go
```

Default flags (see `gserver/main.go`): `-http=:8080`, `-https=:8443`, `-root=.`, `-timeout=10`.

## Dependencies

- `github.com/rveen/golib` — fn, fs, document, id packages
- `github.com/rveen/ogdl` — graph/template engine
- `github.com/rveen/session` — session management
- `github.com/rveen/certmagic` — TLS (fork with `Shutdown()`)
- `github.com/DATA-DOG/fastroute` — HTTP router
