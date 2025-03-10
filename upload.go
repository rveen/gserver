package gserver

import (
	"crypto/md5"
	"encoding/hex"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rveen/ogdl"
)

const (
	FileDir = "file"
	TmpDir  = ".tmp"
)

func init() {
//	os.MkdirAll(FileDir, 0644)
	os.MkdirAll(TmpDir, 0644)
}

func fileUpload(r *http.Request, user string) (*ogdl.Graph, error) {

	// Handle file uploads. We call ParseMultipartForm here so that r.Form[] is
	// initialized. If it isn't a multipart this gives an error.
	err := r.ParseMultipartForm(10000000) // 10M
	if err != nil {
		return nil, err
	}

	folder := TmpDir

	buf := make([]byte, 1000000)

	var file multipart.File
	var wfile *os.File
	var n int

	g := ogdl.New(nil)

	for k := range r.MultipartForm.File {

		vv := r.MultipartForm.File[k]

		for _, v := range vv {

			file, err = v.Open()
			if err != nil {
				return nil, err
			}
			defer file.Close()

			ext := filepath.Ext(v.Filename)
			if ext == "" {
				// Cannot handle files without extension
				continue
			}

			log.Println("uploading:", folder+"/"+v.Filename)

			wfile, err = os.Create(folder + "/" + v.Filename)
			if err != nil {
				return nil, err
			}
			defer wfile.Close()

			h := md5.New()
			for {
				n, err = file.Read(buf)
				if n > 0 {
					wfile.Write(buf[:n])
					h.Write(buf[:n])
				}
				if err != nil {
					break
				}
			}

			fname := FileDir + "/" + hex.EncodeToString(h.Sum(nil)) + ext

			log.Println("uploading file with MD5", hex.EncodeToString(h.Sum(nil)))
			log.Println("moving to", fname)
			os.Rename(folder+"/"+v.Filename, fname)

			f := g.Add("-")
			f.Add("path").Add(fname)
			f.Add("name").Add(v.Filename[:len(v.Filename)-len(ext)])
			f.Add("fullname").Add(v.Filename)

		}
	}

	return g, nil
}
