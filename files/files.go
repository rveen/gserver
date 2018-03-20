// Access to files
//
// Features:
// - trailing slash and index file
// - file extension is optional
// - preprocess templates
// - variables in path
//
// The problem
//
// domain/*static
// domain/:user/*static
// domain/:user/static/:id/*static
//
// Examples
//
// domain/:user1/prj/:pid/event/:eid
//
package files

import (
	"errors"
	"io/ioutil"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/russross/blackfriday"
	"github.com/rveen/ogdl"
)

type Files struct {
	Root      string
	GetConfig func(reqctx *ogdl.Graph, par string, level int) *ogdl.Graph
}

type FileEntry struct {
	Path    string
	Mime    string
	Content []byte
	Tree    *ogdl.Graph
	Type    string
}

var isTemplate = map[string]bool{
	".htm": true,
	".txt": true,
}

func New(root string) *Files {
	root, _ = filepath.Abs(root)

	log.Println("gserver.Files.New", root)
	return &Files{Root: root}
}

func (f *FileEntry) getFile(path string) {

	var err error

	// log.Printf("Files.getFile(%s)", path)

	f.Content, err = ioutil.ReadFile(path)
	if err != nil {
		return
	}

	f.Type = "b"

	ext := filepath.Ext(path)

	// set MIME type
	f.Mime = mime.TypeByExtension(ext)

	// Pre-process template or markdown
	if isTemplate[ext] {
		f.Tree = ogdl.NewTemplate(string(f.Content))
		f.Type = "t"
	} else if ext == ".md" {
		// Process markdown
		f.Content = blackfriday.MarkdownCommon(f.Content)
		f.Mime = "text/html"
		f.Type = "m"
	}

	// log.Printf("Files.getFile: len %d bytes, type %s ext %s\n", len(f.Content), f.Type, ext)
}

func (f *Files) Get(path string, filterUnderscore bool, ctx *ogdl.Graph) (*FileEntry, map[string]string, error) {

	log.Println("files.Get:" + path)

	// Prepare and clean path
	path = filepath.Clean(path)
	if path == "/" {
		path = "/index.htm"
	}

	// We should be in the root directory
	err := os.Chdir(f.Root)
	if err != nil {
		return nil, nil, err
	}

	parts := strings.Split(path, "/")
	path = ""
	dir := "."
	fe := &FileEntry{}

	v := make(map[string]string)
	n := -1

	for i := 0; i < len(parts); i++ {
		n++

		part := parts[i]

		// protection agains starting slash
		if part == "" {
			continue
		}

		if part[0] == '.' {
			return nil, nil, errors.New("path element starting with . not allowed (see files.go)")
		}

		if filterUnderscore && part[0] == '_' {
			return nil, nil, errors.New("path element starting with _. Currently not allowed (see files.go)")
		}

		if path == "" {
			path = part
		} else {
			path += "/" + part
		}
	again:
		file, err := os.Stat(path)
		log.Println("files.Get stat:", path, dir)

		if err != nil {
			// Could exist with some extension extension or be a parameter

			// Open dir

			d, err := ioutil.ReadDir(dir)
			if err != nil {
				return nil, v, err
			}

			base := filepath.Base(path)

			// any files with extension ?
			for _, fi := range d {
				name := fi.Name()

				if name == base+".htm" {
					path = path + ".htm"
					goto again
				}
			}

			// any entry starting with _ ?

			for _, fi := range d {
				name := fi.Name()

				if name[0] == '_' {

					r := f.inList(ctx, name, part, parts, n)
					if r != "" {
						v["path"] = r
						path = dir + "/" + name + "/path"
						i = len(parts)
					} else {
						v[name[1:]] = part
						path = dir + "/" + name
					}
					goto again
				}
			}

			return nil, v, nil
		}

		if !file.IsDir() {
			fe.getFile(path)
			return fe, v, nil
		}
		dir = path
		fe.Path = path
		fe.Type = "d"
		// Dir: continue
	}

	// Check for index file if dir

	if fe.Type == "d" {
		fe.getFile(fe.Path + "/index.htm")
	}

	return fe, v, nil
}

func (f *Files) inList(ctx *ogdl.Graph, name, part string, parts []string, n int) string {

	if f.GetConfig == nil {
		return ""
	}

	// Load the domain configuration if any
	ctx.Set("R."+name[1:], part)
	g := f.GetConfig(ctx, name[1:], n)

	log.Println("inList", name, part, n)

	if g == nil || len(parts) <= n+1 {
		return ""
	}

	list := g.Get("staticURLs")

	if list == nil {
		return ""
	}

	log.Println("inList (2)")

	// TODO esto es caca
	remaining := parts[n+1]
	for j := n + 2; j < len(parts); j++ {
		remaining += "/" + parts[j]
	}

	if list.Node(remaining) == nil {
		return ""
	}

	// Found ! path is in domain's 'static' list
	// the rest of the parts are substituted by one: path
	return remaining
}
