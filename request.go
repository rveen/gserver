package gserver

import (
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/chmike/securecookie"
	"github.com/rveen/golib/fn"
	"github.com/rveen/ogdl"
	"github.com/rveen/session2"
)

type Request struct {
	HttpRequest *http.Request
	User        string
	Context     *ogdl.Graph
	Params      *ogdl.Graph
	Path        string
	File        *fn.FNode
	Mime        string
	// Session is nil for anonymous requests: a session is only stored once a
	// user authenticates.
	Session *session2.Session
}

var TplExtensions []string = []string{".htm", ".txt", ".csv", ".json", ".g", ".ogdl", ".xml", ".xlsx", ".svg", ".ics"}

func ConvertRequest(r *http.Request, w http.ResponseWriter, host bool, srv *Server) *Request {

	rq := &Request{HttpRequest: r}

	var s *session2.Session

	rq.Context, s = getSession(r, w, host, srv)
	if rq.Context == nil {
		// Could not create a new session
		return nil
	}
	rq.User = rq.Context.Get("user").String()
	rq.Session = s

	// Add host name in case of multihost
	if host {
		rq.Path = r.Host + "/" + r.URL.Path
	} else {
		rq.Path = r.URL.Path
	}

	rq.Path = filepath.Clean(rq.Path)

	// load a pointer to a copy: do not touch srv.Root.
	// Achtung!!: this doesn't work: rq.File = &(*srv.Root)
	f := *srv.Root
	rq.File = &f

	return rq
}

func getSession(r *http.Request, w http.ResponseWriter, host bool, srv *Server) (*ogdl.Graph, *session2.Session) {

	// May be nil, and that is the normal case: anonymous requests get no stored
	// session. One is created lazily below, only once an authenticated user is
	// known. See ensure().
	sess := session2.Get(r)

	// Build a per-request overlay: local nodes (user, userACL, R.*) shadow the
	// shared read-only server context without copying it.
	srv.ContextMu.RLock()
	var parent *ogdl.Graph
	if !host {
		parent = srv.Context
	} else {
		parent = srv.HostContexts[r.Host]
	}
	srv.ContextMu.RUnlock()

	if parent == nil {
		// Multihost with an unknown Host header: there is no context to overlay.
		log.Println("no context for host:", r.Host)
		return nil, nil
	}

	sc := newSessionContext(parent)

	// ensure returns the stored session, creating and registering it on first
	// use. Only ever called once an authenticated user is known, so that an
	// anonymous flood cannot fill the session table.
	ensure := func() *session2.Session {
		if sess == nil {
			sess = session2.NewSession(session2.SessOptions{Timeout: srv.SessionTimeout})
			session2.Add(sess, w)
		}
		return sess
	}

	// Restore session-persistent scalars
	if sess != nil {
		if u, ok := sess.Attr("user").(string); ok && u != "" {
			sc.Set("user", u)
		}
		if a, ok := sess.Attr("userACL").(string); ok && a != "" {
			sc.Set("userACL", a)
		}
	}

	// Iff the userCookie is set, set 'user' to its value
	user := UserCookieValue(r)
	if user != "" && user != "-" {
		sc.Set("user", user)
		ensure().SetAttr("user", user)
	}

	// An externally-resolved identity (bearer token / trusted header) injected
	// by an upstream Authenticate middleware takes precedence over the session
	// cookie. See authbridge.go.
	if iu := userFromContext(r.Context()); iu != nil && iu.UID != "" {
		user = iu.UID
		sc.Set("user", user)
		ensure().SetAttr("user", user)
		if iu.ACL != "" {
			sc.Set("userACL", iu.ACL)
			ensure().SetAttr("userACL", iu.ACL)
		}
	}

	// If there is no user set and there is an auto-login user defined.
	// Deliberately does not touch `user` below, so an auto-login deployment
	// still allocates no session for anonymous traffic.
	u := sc.Node("user").String()
	if (u == "" || u == "nobody") && srv.DefaultUser != "" {
		sc.Set("user", srv.DefaultUser)
	}

	// Set ACL. This can be done better (also set in LoginAdapter
	// TODO move this to LoginAdapter (which is run anyway every request)
	acl := ""
	if user != "" && user != "nobody" {
		acl = sc.Get("userACL").String()
		if acl == "" {
			acl = GetACL(user, srv)
			if acl == "" {
				acl = "-"
			}
			sc.Set("userACL", acl)
			ensure().SetAttr("userACL", acl)
		}
	}

	// Add request specific parameters
	data := sc.Create("R")
	ur := r.URL.Path
	if ur == "" {
		ur = "/"
	}
	data.Set("url", ur)
	data.Set("home", srv.Root.Root)

	r.ParseForm()

	// Add GET, POST, PUT parameters into context
	for k, vv := range r.Form {

		var n *ogdl.Graph

		for _, v := range vv {

			v = removeControlChars(v)

			if strings.HasSuffix(k, "._ogdl") {
				k = k[0 : len(k)-6]
				g := ogdl.FromString(v)
				if n == nil {
					data.Set(k, g)
					n = data.Get(k)
				} else {
					n.Add(g)
				}
			} else {
				if n == nil {
					data.Set(k, v)
					n = data.Get(k)
				} else {
					n.Add(v)
				}
			}
		}
	}

	return sc.Graph(), sess
}

// Remove control characters except TAB and LN
func removeControlChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for _, r := range s {
		// Keep TAB (0x09) and LF (0x0A)
		if r == 0x09 || r == 0x0A {
			b.WriteRune(r)
			continue
		}

		// Filter control characters
		if (r >= 0x00 && r <= 0x1F) || (r >= 0x7F && r <= 0x9F) {
			continue
		}

		b.WriteRune(r)
	}

	return b.String()
}

// Get is a direct map from URL to file (binary content + params)
// TODO: Clean r.Path vs r.File.Path dicotomy
func (r *Request) Get() error {
	// Get the file

	var err error

	// log.Printf("URL.Path (0): %s\n", r.Path)

	if r.HttpRequest.FormValue("m") == "raw" {
		err = r.File.GetRaw(r.Path)
	} else {
		err = r.File.Get(r.Path)
	}

	if err != nil {
		return err
	}

	// Set R.urlbase (for setting <base href="$R.urlbase"> allowing relative URLs)
	base := r.HttpRequest.URL.Path
	// log.Printf("URL.Path: %s\n", base)

	if r.File.Type != "dir" && strings.HasPrefix(r.File.Path, r.File.Root) {
		base = filepath.Dir(r.File.Path[len(r.File.Root):])
	}
	base = strings.ReplaceAll(base, "\\", "/")
	if len(base) > 1 && base[len(base)-1] != '/' {
		base += "/"
	}

	// log.Printf("URL.Path (3): %s\n", base)

	data := r.Context.Node("R")
	data.Set("urlbase", base)

	// Add parameters found in the file path (_token)
	for k, v := range r.File.Params {
		data.Set(k, v)
	}
	return nil
}

// Process processes templates, remaining path fragments and sets mime the type
func (r *Request) Process(srv *Server) error {
	// Process templates
	//
	// Some types have predefined templates, some ARE templates. Predefined
	// templates are taken from the main context, while the content (this
	// path) is injected into the context so that the template can pick it up.

	// Process 'dir', 'file', 'document', 'data' or 'log'
	// If !raw:
	// If file, process .htm and .txt as templates
	// If dir, document or data, use corresponding template
	// When there is a readme, type is 'dir' and fn.Content is not empty

	r.Mime = ""

	if r.HttpRequest.FormValue("m") != "raw" {
		switch r.File.Type {

		case "document", "document_dir":
			r.File.Document.Context = r.Context
			r.File.Content = []byte(r.File.Document.Html())
			fallthrough
		case "dir", "data", "log":
			// log.Println("r.Process: ", r.File.Type)

			r.Context.Set("path.content", string(r.File.Content))
			r.Context.Set("path.data", r.File.Data)

			tp := r.HttpRequest.FormValue("t")
			if tp == "" {
				if strings.HasSuffix(r.File.Path, "readme.md") {
					tp = "readme"
				} else {
					tp = r.File.Type
				}
			}

			tpl := srv.Templates[tp]
			if tpl == nil {
				log.Println("no template for type", tp)
			}
			r.File.Content = tpl.Process(r.Context)
			r.Mime = "text/html"

		default: // 'file'. Check if it is a template
			if hasTplExtension(r.File.Path) {
				// tpl := ogdl.NewTemplate(string(r.File.Content))
				tpl := ogdl.NewTemplateFromBytes(r.File.Content)

				r.Context.Set("mime", "")
				r.File.Content = tpl.Process(r.Context)

				// Allow templates to return arbitrary mime types
				mime := r.Context.Get("mime").String()
				if mime != "" {
					r.Mime = mime
					r.File.Content = r.Context.Get("content").Bytes()
				}
			}
		}
	} else {
		// raw content with template
		if r.HttpRequest.FormValue("t") != "" {
			tpl := srv.Templates[r.HttpRequest.FormValue("t")]
			r.File.Content = tpl.Process(r.Context)
			r.Mime = "text/html"
		}
	}

	// Set Content-Type (MIME type)
	if r.Mime == "" {
		ext := filepath.Ext(r.File.Path)
		r.Mime = mime.TypeByExtension(ext)
	}

	return nil
}

func hasTplExtension(s string) bool {
	for _, v := range TplExtensions {
		if strings.HasSuffix(s, v) {
			return true
		}
	}

	return false
}

func UserCookie() *securecookie.Obj {

	// Key is configurable via SetUserCookieKey (see authbridge.go).
	userCookie := securecookie.MustNew("userid", userCookieKey, securecookie.Params{
		Path:   "/",
		MaxAge: 0,
		Secure: false, // cookie received with HTTP for testing purpose
	})
	return userCookie
}

func UserCookieValue(r *http.Request) string {

	userCookie := UserCookie()

	b, err := userCookie.GetValue(nil, r)
	if err != nil {
		return ""
	}
	return string(b)
}

func DeleteUserCookie(w http.ResponseWriter) {

	c := &http.Cookie{
		Name:     "userid",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	}

	http.SetCookie(w, c)
}

// RedirectCookie holds the post-login destination across the login form POST.
// It is a signed cookie rather than a session attribute because /login is
// reached anonymously, and stashing it server-side would let any unauthenticated
// client fill the session table.
func RedirectCookie() *securecookie.Obj {

	return securecookie.MustNew("redirect", userCookieKey, securecookie.Params{
		Path:     "/",
		MaxAge:   600,
		HTTPOnly: true,
		Secure:   false, // cookie received with HTTP for testing purpose
	})
}

func SetRedirectCookie(w http.ResponseWriter, path string) {
	RedirectCookie().SetValue(w, []byte(path)) //nolint:errcheck
}

func RedirectCookieValue(r *http.Request) string {

	b, err := RedirectCookie().GetValue(nil, r)
	if err != nil {
		return ""
	}
	return string(b)
}

func DeleteRedirectCookie(w http.ResponseWriter) {

	c := &http.Cookie{
		Name:     "redirect",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	}

	http.SetCookie(w, c)
}

// safeRedirect confines a post-login redirect to this host. A bare "//host"
// or an absolute URL would otherwise send the user off-site.
func safeRedirect(p string) string {
	if strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") {
		return p
	}
	return "/"
}
