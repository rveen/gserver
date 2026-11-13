# gserver, in Go

gserver is an experimental web server adapted to serve OGDL templates along with
static content. It is not optimized and not well tested, as of now.

gserver serves plain HTTP only. For HTTPS, run it behind a proxy that
terminates TLS (Caddy, Apache httpd, nginx); see [Behind a proxy](#behind-a-proxy).

## Features

 - The file extension of (some) files below the document root is optional
 - Trailing slash and index files detection
 - Markdown rendering on the fly.
 - Login, Logout example functionality
 - Upload files go to /_user/file/$user/$folder/
 - Template functionality extensible through Go plugins

## License

MIT (see LICENSE).

## As a library

The module path is `github.com/rveen/gserver/v3` (v3 moved to ogdl v2;
v2.0.0 and v1.0.0 remain for existing users). A program serving a site is a few lines:

```go
import (
	"flag"
	"log"
	"path/filepath"

	"github.com/rveen/gserver/v3"
	_ "github.com/rveen/gserver/v3/context/plugins/htmlctx" // template objects
)

func main() {
	var o gserver.Options
	o.BindFlags(flag.CommandLine) // -root -H -t -ts -ms -userdb -l -log -v
	flag.Parse()
	// The `server` section of the site's config, for what flags did not set.
	if err := o.ApplyConfig(filepath.Join(o.Root, ".conf/config.ogdl")); err != nil {
		log.Fatal(err)
	}
	srv, err := gserver.Open(o)
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(gserver.Run(srv, srv.Mux())) // until SIGTERM / SIGINT
}
```

`Options` also takes `Assets` (serve the site from embedded files),
`CheckPassword` (your own login check) and `DocFilter`. A program with its own
router wires `srv.Handlers()` instead of `srv.Mux()`. Template objects are
plugins: a package registers a factory with `context/ctxreg` in its `init`,
and a blank import enables it. `gserver/main.go` is the reference server.

## Running

gserver serves the site in its working directory, or the one given with `-root`.
That directory needs `.conf/config.ogdl` and `.conf/context.ogdl`; all other
paths (`.conf/*`, uploads, documents) are relative to it.

    cd gserver && sh build       # static binary (CGO_ENABLED=0)
    ./gserver -root /srv/gserver/mysite -H :8080

| Flag | Default | Meaning |
|---|---|---|
| `-root` | current dir | site directory |
| `-H` | `:80` | HTTP host:port |
| `-t` | `10` | HTTP read/write timeout (seconds) |
| `-ts` | `30` | session timeout (minutes) |
| `-ms` | `0` | max concurrent logged-in sessions (0 = default) |
| `-userdb` | `htaccess` | user database: `htaccess` or `sql` (`sqlite` is the same) |
| `-l` | stderr | log file (appended to) |
| `-m` | `false` | multiple hosts (one subdirectory per host name) |
| `-log` | `true` | logging on/off |
| `-v` | `false` | no effect; kept for compatibility |

On SIGTERM or SIGINT gserver lets running requests finish and exits with status 0.
Any startup or listener error is printed to stderr (even with `-log=false`) and
exits with status 1. When started by systemd, log lines carry no timestamp, since
journald adds its own.

### Login cookie key

The login cookie is signed with a secret key. Set it with the environment
variable `GSERVER_COOKIE_KEY` (any string; use a long random one, e.g.
`openssl rand -base64 32`). Without it gserver uses a random key for each
run, logs a warning, and every login ends when the server restarts.

Keep the key out of the site directory and the command line; with systemd,
put it in an `EnvironmentFile` readable only by root (see the unit file).

### Server options in .conf/config.ogdl

Every flag except `-root` can also be set in a `server` section of
`.conf/config.ogdl`, so that the command line can stay the same for all sites:

    server
      host 127.0.0.1:82
      timeout 30
      session_timeout 60

| Key | Flag |
|---|---|
| `host` | `-H` |
| `timeout` | `-t` |
| `session_timeout` | `-ts` |
| `max_sessions` | `-ms` |
| `userdb` | `-userdb` |
| `multi` | `-m` |
| `log` | `-log` |
| `verbose` | `-v` |

A flag given on the command line overrides the config file. An unknown key or an
invalid value stops the server at startup. The keys `secure_host`, `email` and
`certs`, which configured the former built-in HTTPS, stop it with a message
saying to remove them and terminate HTTPS in a proxy.

### systemd service

`gserver/systemd/gserver@.service` is a template unit: one instance per site,
`gserver@mysite` serving `/srv/gserver/mysite`. It runs as user `gserver` in
Apache's group `www`, may bind ports below 1024 without root
(`CAP_NET_BIND_SERVICE`) and restarts 30 s after a failure. All other options come from the site's
`.conf/config.ogdl`.

The group lets gserver and Apache share files (group-readable, or
group-writable where both write) without running as the same user. `www` is
Apache's group on SUSE; on other distributions change `Group=` to `www-data`
(Debian, Ubuntu) or `apache` (RHEL, Fedora), or the service fails to start
with status `216/GROUP`.

An existing `gserver` user can be moved to the group with
`sudo usermod -g www gserver`; then re-own its files
(`sudo chown -R gserver:www /srv/gserver/mysite /var/lib/gserver`).

Installation:

    sudo cp gserver/gserver /usr/local/bin/
    sudo useradd -r -g www -d /var/lib/gserver -s "$(command -v nologin)" gserver
    sudo install -d -o gserver -g www /var/lib/gserver
    sudo chown -R gserver:www /srv/gserver/mysite
    sudo cp gserver/systemd/gserver@.service /etc/systemd/system/
    sudo systemctl daemon-reload
    sudo systemctl enable --now gserver@mysite

Then:

    systemctl status gserver@mysite
    journalctl -u gserver@mysite -f
    sudo systemctl restart gserver@mysite    # after changing config.ogdl

The unit locks the file system down to read-only (`ProtectSystem=strict`), except
for the site directory and `/var/lib/gserver`. If the site writes elsewhere (e.g.
repositories outside the site directory), add those paths to `ReadWritePaths=`,
preferably in an override (`systemctl edit gserver@mysite`) rather than in the
unit file.

The unit needs systemd 232 or later (e.g. SUSE Linux Enterprise 15, RHEL 8,
Debian 10, Ubuntu 18.04). Things to check on the target machine:

 - if gserver is reached directly (not through a proxy), the firewall must allow
   HTTP, e.g. with firewalld:
   `firewall-cmd --permanent --add-service=http && firewall-cmd --reload`
 - no other process may already be listening on gserver's port:
   `ss -ltnp | grep -E ':80\b'`
 - build for the target's architecture, e.g. `GOARCH=arm64 sh build`

### Behind a proxy

For HTTPS, gserver runs behind a reverse proxy (Caddy, Apache httpd, nginx)
that terminates TLS. gserver then serves plain HTTP on a local port, here 82.

With Caddy, which obtains and renews certificates itself, the whole proxy
configuration is:

    www.example.com {
        reverse_proxy 127.0.0.1:82
    }

Caddy keeps the `Host` header and sets `X-Forwarded-For` by default.

The rest of this section uses Apache.

In the site's `.conf/config.ogdl`:

    server
      host 127.0.0.1:82
      timeout 300

 - Listen on `127.0.0.1`, not `:82`, so that gserver can only be reached
   through Apache.
 - `timeout` is gserver's read/write timeout for a whole request. The default of
   10 seconds cuts off slow uploads and downloads, which Apache reports as
   502 errors. Keep it at or below Apache's `ProxyTimeout`.

The systemd unit can stay as it is. Port 82 is below 1024, so it still needs
`CAP_NET_BIND_SERVICE`; with a port above 1024 those lines can be removed.

On the Apache side (SUSE Linux Enterprise 15; Apache runs as `wwwrun:www`):

    sudo a2enmod proxy
    sudo a2enmod proxy_http
    sudo a2enmod headers

and in the HTTPS virtual host, e.g. `/etc/apache2/vhosts.d/mysite.conf`:

    <VirtualHost *:443>
        ServerName www.example.com
        # SSLEngine on, SSLCertificateFile ... as usual

        ProxyPreserveHost On
        ProxyPass        / http://127.0.0.1:82/
        ProxyPassReverse / http://127.0.0.1:82/
        ProxyTimeout 300
        RequestHeader set X-Forwarded-Proto "https"
    </VirtualHost>

followed by `sudo systemctl reload apache2`.

 - `ProxyPreserveHost On` is required in multi-host mode (`-m`), where gserver
   selects the site from the `Host` header. Without it every request arrives as
   `127.0.0.1:82`.
 - gserver's redirects are relative and its cookies work over plain HTTP, so
   nothing else needs rewriting.
 - Only Apache's ports (80/443) need to be open in the firewall, not 82.

Client addresses: Apache adds the client's address to the `X-Forwarded-For`
header. gserver logs that address instead of the proxy's, but only for requests
coming from a loopback address (127.0.0.0/8, ::1), so that remote clients
cannot fake it. The last entry of the header is used, since earlier entries
can be set by the client.

## Parameter substitution in paths

While resolving a path in the document root, entries of the form _token are
used for path elements not directly found in the file system. In that case 'token'
will be available later as a variable, set to the unknown path element.

For example:

     /john/blog/1

will be sent to

    /_user/blog/_id/index.htm

if that path is present. Two variables will be available in the context:

    user=john
    id=1

## Routes

There are two routes configured in gserver:

    /:user/file/*filepath
	/*filepath

The first one goes to a static file handler that translates that path to
/_user/file/:user/*filepath. This handler doesn't create or need sessions.

The second goes to a handler that uses the parameter substitution
mechanism explained above, creates sessions, processes file uploads and templates.

Any path that has not the form /token/file/* goes to this second route.

The static handler will not return paths with elements that start with a dot.
The dynamic handler, in addition, will ignore path with elements starting with an
underscore, since these are reserved for variables.

## File upload

Any POST request with files attached (multipart) and a field named "UploadFiles"
that is handled by the second handler will store those files in /_user/file/$user/$folder/.
$user is the current authenticated user and $folder is 'default' or the content of
a field named 'folder' in the request.

Without an authenticated user no files will be stored in the server.

TODO: public and private files (ACL, casbin?)

## Templates

TODO

## Remote functions

OGDL remote functions (RPC endpoints) can be configured in .conf/config:

    ogdlrf
      git
        host localhost:1135
      zoekt
        host localhost:1166

TODO: document in ogdl-go how to create servers and clients.

## Plugins

Plugins that are present in .conf/plugin are loaded in made available in the
request context. Public methods in those plugins can then be accessed in templates.
If a plugin database.so is present, a Database object is expected which will be
placed in the context so that $database.Method() can be called in the template.

### Request interceptors

Packages registered through `golib/fn/httphook` handle a request before normal
file resolution, after the login check. They are enabled by a blank import in
`gserver/main.go`; removing the line removes the dependency.

- `golib/formats/altium/plugin` — `Foo.SchDoc.kicad_sch` (resp.
  `Foo.PcbDoc.kicad_pcb`) is the Altium file converted to KiCad on the fly.
- `golib/formats/nomo/plugin` — a `.nomo` worksheet is served as a typeset HTML
  page, evaluated by Nomo's own WebAssembly engine (run in-process by wazero).
  `?m=raw` returns the source. The math font is served at
  `/.nomo/stix-two-math-subset.woff2`, its licence at `/.nomo/OFL.txt`. Links
  must carry the `.nomo` extension; an extensionless path is not intercepted.

## Markdown processor ($MD())

TODO: describe the extensions and the \escape inline syntax for processing style
and function calls.

## Form to context

Form inputs are stored in the request's context and made directly accessible in 
templates. Input names are taken as simple paths (tokens separated by dots), and 
the special case where the name ends with ._ogdl is parsed as OGDL before stored 
in the context.

For example, the content of the following form:

    <form>
	<input name="obj.name" />
	<input name="obj.conf._ogdl"/>

could be something like this:

    obj
      name
        "Pepe Delgado"
      conf
        ip
          192.160.1.1
        net
          255.255.255.0

## Login, Logout

There is no specific path for login or logout. Any request which does not go to
the static file handler will recognize the parameters 'Login' and 'Logout'
if present. The default login handler accepts any non-empty username and any password.

The 'redirect' parameter can be used to send the user to a specific page after
login. The default behavior is to return to the same page.




