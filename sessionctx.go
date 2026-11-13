package gserver

import "github.com/rveen/ogdl/v2"

// SessionContext is a lightweight per-request overlay over the server's read-only
// context. Only session-specific values (user, userACL, R.*) are stored locally;
// everything else resolves from Parent without copying.
type SessionContext struct {
	local  *ogdl.Node
	Parent *ogdl.Node // points to srv.Context or a host context; never written
}

func newSessionContext(parent *ogdl.Node) *SessionContext {
	return &SessionContext{local: ogdl.New(nil), Parent: parent}
}

func (sc *SessionContext) Set(key string, val interface{}) { sc.local.Set(key, val) }

func (sc *SessionContext) Get(key string) *ogdl.Node {
	if n := sc.local.Lookup(key); n != nil && n.Len() > 0 {
		return n
	}
	return sc.Parent.Lookup(key)
}

func (sc *SessionContext) Node(key string) *ogdl.Node {
	if n := sc.local.Node(key); n != nil {
		return n
	}
	return sc.Parent.Node(key)
}

func (sc *SessionContext) Create(key string) *ogdl.Node { return sc.local.Create(key) }

// Graph builds a merged *ogdl.Graph for the template engine.
// Local nodes come first so they shadow parent entries of the same name.
// Parent entries are wrapped in shell nodes so writes during template processing
// cannot reach the shared srv.Context.
func (sc *SessionContext) Graph() *ogdl.Node {
	merged := ogdl.New(nil)
	merged.AddNodes(sc.local)
	for _, n := range sc.Parent.Out {
		merged.Out = append(merged.Out, &ogdl.Node{Data: n.Data, Out: n.Out})
	}
	return merged
}
