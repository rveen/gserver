package gserver

import (
	"fmt"
	"io/ioutil"
	"log"
	"plugin"
	"reflect"
	"strings"
	"unsafe"

	"github.com/icza/session"
	"github.com/rveen/gserver/files"
	"github.com/rveen/ogdl"
	rpc "github.com/rveen/ogdl/ogdlrf"
)

type Server struct {
	Host       string
	SecureHost string
	Config     *ogdl.Graph
	Context    *ogdl.Graph
	Root       *files.Files
	DocRoot    string
	UploadDir  string
	Sessions   session.Manager
	Plugins    []string
}

// New prepares a Server{} structure initialized with
// configuration information and a base context that will be
// the initial context of each request.
//
func New() (*Server, error) {

	srv := Server{}

	// DocRoot has to end with a slash
	srv.DocRoot = "./"

	// UploadDir has to end with a slash
	srv.UploadDir = "files/"

	// Default host
	srv.Host = ":8080"

	// Server configuration file (optional)
	srv.Config = ogdl.FromFile(".conf/config.g")
	if srv.Config == nil {
		srv.Config = ogdl.New()
	}

	// Base context for templates (optional)
	srv.Context = ogdl.FromFile(".conf/context.g")

	// Register remote functions
	rfs := srv.Config.Get("ogdlrf")
	if rfs != nil {
		for _, rf := range rfs.Out {
			name := rf.ThisString()
			host := rf.Get("host").String()
			proto := rf.Get("protocol").Int64(2)
			log.Println("remote function registered:", name, host, proto)
			f := rpc.Client{Host: host, Timeout: 1, Protocol: int(proto)}
			srv.Context.Set(name, f.Call)
		}
	}

	// Load plugins

	pluginDir := ".conf/plugin/"

	pp, err := ioutil.ReadDir(pluginDir)
	if err == nil {

		for _, p := range pp {
			if !strings.HasSuffix(p.Name(), ".so") {
				continue
			}
			key := p.Name()[0 : len(p.Name())-3]
			symbol := strings.Title(key)

			pi, err := plugin.Open(pluginDir + p.Name())
			if err == nil {
				log.Println("plugin loaded:", p.Name())
				// inspectPlugin(pi)

				log.Println(" - Expected symbol", symbol)

				fn, err := pi.Lookup(symbol)
				if err == nil {
					log.Println(" - Symbol found! Bind to", key)
					srv.Context.Set(key, fn)
					log.Println(" - Symbol kind", reflect.ValueOf(fn).Kind().String())
					srv.Plugins = append(srv.Plugins, key)
				}
			} else {
				log.Println(err)
			}
		}
	}

	// Session manager
	session.Global.Close()
	srv.Sessions = session.NewCookieManagerOptions(session.NewInMemStore(), &session.CookieMngrOptions{AllowHTTP: true})

	// File cache
	srv.Root = files.New(srv.DocRoot)

	return &srv, nil
}

type Plug struct {
	Path    string
	_       chan struct{}
	Symbols map[string]interface{}
}

func inspectPlugin(p *plugin.Plugin) {
	pl := (*Plug)(unsafe.Pointer(p))

	fmt.Printf("Plugin %s exported symbols (%d): \n", pl.Path, len(pl.Symbols))

	for name, pointers := range pl.Symbols {
		fmt.Printf("symbol: %s, pointer: %v, type: %v\n", name, pointers, reflect.TypeOf(pointers))
	}
}
