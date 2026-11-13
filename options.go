package gserver

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"time"

	"github.com/rveen/ogdl/v2"
)

// Options configure a server opened with Open. The zero value is usable: every
// field has a default, applied by Open.
type Options struct {
	// Root is the site directory. Open changes the working directory to it,
	// since every path the server uses (.conf, .tmp, files/, ../htpasswd) is
	// relative to the site. Default: the current directory.
	Root string

	// Host is the listen address. Default ":80".
	Host string

	// Multi serves one subdirectory per host name (named after the host, such
	// as example.com/), each with its own .conf/context.ogdl.
	Multi bool

	// Timeout is the HTTP read and write timeout. Default 10s.
	Timeout time.Duration

	// SessionTimeout is the idle time after which a login session ends.
	// Default 30m.
	SessionTimeout time.Duration

	// MaxSessions caps the number of stored (logged-in) sessions; past the
	// cap the least recently used one is dropped. 0 keeps the library
	// default of 100000.
	MaxSessions int

	// UserDB selects the password check of the login form: "htaccess" (the
	// default; the file ../htpasswd, relative to the site) or "sql" (the
	// users table of Server.UserDb, which the build must open). "sqlite" is
	// accepted as another name for "sql". Ignored when CheckPassword is set.
	UserDB string

	// CheckPassword, when set, replaces UserDB as the password check of the
	// login form.
	CheckPassword func(user, pass string) bool

	// DocFilter, when set, post-processes the HTML that the context functions
	// DOCV, DocLinksV and docPartV render for a logged-in user. KF uses it to
	// rewrite file links according to the user's rights.
	DocFilter func(html, user string) string

	// Assets serves the site from embedded files instead of Root (optional).
	Assets *Assets

	// LogFile, when set, receives the log instead of stderr. The file is
	// appended to.
	LogFile string

	// NoLog discards the log once the server is set up (flag -log=false).
	NoLog bool

	// Verbose is accepted for command-line compatibility (-v) and has no
	// effect.
	Verbose bool

	// fs is the flag set BindFlags bound to, so that ApplyConfig can tell
	// which flags were given on the command line.
	fs *flag.FlagSet
}

// Assets lets a build serve its site from embedded files instead of Root. Any
// field may be nil; whatever is missing is read from Root. Static replaces
// Root for /static/ and /favicon.ico entirely; uploaded files (/file/,
// /files/) are always read from Root.
//
// Files resolved from Static or Dynamic are trusted: the protected-path check
// (config.ogdl `protected`) and the request interceptors do not run for them.
// A request that misses in Dynamic falls back to Root, with those checks.
type Assets struct {
	Static  fs.FS // served by the static handler (/static/, /favicon.ico)
	Dynamic fs.FS // resolved by the dynamic handler
	Conf    fs.FS // holds .conf/config.ogdl, .conf/context.ogdl, .conf/templates.ogdl
}

// Defaults of Options, applied by Open and shown by BindFlags.
const (
	DefaultHost           = ":80"
	DefaultTimeout        = 10 * time.Second
	DefaultSessionTimeout = 30 * time.Minute
	DefaultUserDB         = "htaccess"
)

// withDefaults returns o with every unset field given its default.
func (o Options) withDefaults() Options {
	if o.Host == "" {
		o.Host = DefaultHost
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.SessionTimeout <= 0 {
		o.SessionTimeout = DefaultSessionTimeout
	}
	if o.UserDB == "" {
		o.UserDB = DefaultUserDB
	}
	return o
}

// BindFlags defines the server's command-line flags on fs, bound to o:
//
//	-root -H -m -t -ts -ms -userdb -l -log -v
//
// A flag's default is the value o already holds, or the library default, so a
// build can change a default by setting the field before calling BindFlags.
// Call ApplyConfig after fs.Parse to fill in what was not given.
func (o *Options) BindFlags(fs *flag.FlagSet) {

	*o = o.withDefaults()
	o.fs = fs

	fs.StringVar(&o.Root, "root", o.Root, "site directory (default: current directory)")
	fs.StringVar(&o.Host, "H", o.Host, "set host:port")
	fs.BoolVar(&o.Multi, "m", o.Multi, "enable multiple hosts (one subdirectory per host name)")
	fs.Var((*seconds)(&o.Timeout), "t", "set http timeout (seconds)")
	fs.Var((*minutes)(&o.SessionTimeout), "ts", "set session timeout (minutes)")
	fs.IntVar(&o.MaxSessions, "ms", o.MaxSessions, "max concurrent logged-in sessions (0 = leave default)")
	fs.StringVar(&o.UserDB, "userdb", o.UserDB, "user db: htaccess or sql")
	fs.StringVar(&o.LogFile, "l", o.LogFile, "log file (default: stderr)")
	fs.Var((*inverted)(&o.NoLog), "log", "turn logging ON/off")
	fs.BoolVar(&o.Verbose, "v", o.Verbose, "no effect; kept for compatibility")
}

// configKeys maps keys of the `server` section of .conf/config.ogdl to flags.
//
//	server
//	  host :8080
//	  timeout 30
var configKeys = map[string]string{
	"host":            "H",
	"multi":           "m",
	"timeout":         "t",
	"session_timeout": "ts",
	"max_sessions":    "ms",
	"userdb":          "userdb",
	"logfile":         "l",
	"log":             "log",
	"verbose":         "v",
}

// removedKeys are keys of the `server` section that configured HTTPS, which
// gserver no longer does. They are an error with an explanation rather than a
// bare "unknown key", so that a site upgraded in place is told what to do.
var removedKeys = map[string]bool{
	"secure_host": true,
	"email":       true,
	"certs":       true,
}

// ApplyConfig sets the options not given on the command line from the
// `server` section of the config file, if the file and the section exist.
// Flags win. Without BindFlags every key in the section applies.
//
// file is a path, usually filepath.Join(o.Root, ".conf/config.ogdl"). An
// unknown key is an error, and so are the keys of the removed HTTPS support.
func (o *Options) ApplyConfig(file string) error {
	b, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	// v1 accepted malformed input silently; v2 reports it, and a config
	// that doesn't parse must not be mistaken for one without options.
	g, err := ogdl.ParseV1(bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return o.applyConfig(file, g)
}

func (o *Options) applyConfig(file string, g *ogdl.Node) error {

	cfg := g.Lookup("server")
	if cfg == nil {
		return nil
	}

	fs := o.fs
	given := map[string]bool{}
	if fs == nil {
		// Not bound to a command line: bind to a private set, so that values
		// are parsed exactly as flags would be.
		fs = flag.NewFlagSet("config", flag.ContinueOnError)
		o.BindFlags(fs)
		o.fs = nil
	} else {
		fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	}

	for _, n := range cfg.Out {
		key := n.ThisString()
		if removedKeys[key] {
			return fmt.Errorf("%s: server.%s is no longer supported: gserver serves plain HTTP only; terminate HTTPS in a proxy (Caddy, Apache httpd, nginx) and remove this key", file, key)
		}
		name, ok := configKeys[key]
		if !ok {
			return fmt.Errorf("%s: unknown key server.%s", file, key)
		}
		if given[name] {
			continue
		}
		if err := fs.Set(name, n.String()); err != nil {
			return fmt.Errorf("%s: server.%s: %v", file, key, err)
		}
	}
	return nil
}

// seconds is a time.Duration flag given in whole seconds.
type seconds time.Duration

func (d *seconds) String() string { return strconv.Itoa(int(time.Duration(*d) / time.Second)) }
func (d *seconds) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*d = seconds(time.Duration(n) * time.Second)
	return nil
}

// minutes is a time.Duration flag given in whole minutes.
type minutes time.Duration

func (d *minutes) String() string { return strconv.Itoa(int(time.Duration(*d) / time.Minute)) }
func (d *minutes) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*d = minutes(time.Duration(n) * time.Minute)
	return nil
}

// inverted is a boolean flag stored negated: -log=false sets NoLog.
type inverted bool

func (b *inverted) String() string   { return strconv.FormatBool(!bool(*b)) }
func (b *inverted) IsBoolFlag() bool { return true }
func (b *inverted) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	*b = inverted(!v)
	return nil
}
