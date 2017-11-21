package gserver

import (
	"gserver/files"
	"log"

	"github.com/icza/session"
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
			log.Println("remote function registered:", name, host)
			f := rpc.Client{Host: host, Timeout: 1}
			srv.Context.Set(name, f.Call)
		}
	}

	// Session manager
	session.Global.Close()
	srv.Sessions = session.NewCookieManagerOptions(session.NewInMemStore(), &session.CookieMngrOptions{AllowHTTP: true})

	// File cache
	srv.Root = files.New(srv.DocRoot)

	return &srv, nil
}
