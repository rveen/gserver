package gserver

import (
	"crypto/md5"
	"encoding/hex"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

func fileUpload(r *http.Request, user string) ([]string, error) {

	/*
		if user == "nobody" || len(user) == 0 {
			return errors.New("user not logged in")
		}
	*/

	// Handle file uploads. We call ParseMultipartForm here so that r.Form[] is
	// initialized. If it isn't a multipart this gives an error.
	err := r.ParseMultipartForm(10000000) // 10M
	if err != nil {
		return nil, err
	}

	folder := filepath.Clean("_tmp")
	os.MkdirAll(folder, 644)

	buf := make([]byte, 1000000)

	var file multipart.File
	var wfile *os.File
	var n int

	var ff []string

	for k := range r.MultipartForm.File {

		vv := r.MultipartForm.File[k]

		for _, v := range vv {

			file, err = v.Open()
			if err != nil {
				return nil, err
			}
			defer file.Close()

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

			fname := "file/hash/" + hex.EncodeToString(h.Sum(nil)) + filepath.Ext(v.Filename)

			log.Println("uploading file with MD5", hex.EncodeToString(h.Sum(nil)))
			log.Println("moving to", fname)
			os.Rename(folder+"/"+v.Filename, fname)

			ff = append(ff, fname)

		}
	}

	return ff, nil
}
