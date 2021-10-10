package gserver

import (
	"errors"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func fileUpload(r *http.Request, user string) error {

	/*
		if user == "nobody" || len(user) == 0 {
			return errors.New("user not logged in")
		}
	*/

	// Handle file uploads. We call ParseMultipartForm here so that r.Form[] is
	// initialized. If it isn't a multipart this gives an error.
	err := r.ParseMultipartForm(10000000) // 10M
	if err != nil {
		return err
	}

	// Where to store the file
	folder := r.FormValue("folder")
	folder = filepath.Clean(folder)
	log.Println("upload to folder", folder)

	if len(folder) > 64 || strings.Contains(folder, "..") {
		return errors.New("incorrect folder name " + folder)
	}
	folder = filepath.Clean("_user/file/" + user + "/" + folder + "/")

	os.MkdirAll(folder, 644)
	buf := make([]byte, 1000000)
	log.Println("folder for uploading:", folder)

	var file multipart.File
	var wfile *os.File
	var n int

	for k := range r.MultipartForm.File {

		vv := r.MultipartForm.File[k]

		for _, v := range vv {

			file, err = v.Open()
			if err != nil {
				return err
			}
			defer file.Close()

			log.Println("uploading:", folder+"/"+v.Filename)

			wfile, err = os.Create(folder + "/" + v.Filename)
			if err != nil {
				return err
			}
			defer wfile.Close()

			for {
				n, err = file.Read(buf)
				if n > 0 {
					wfile.Write(buf[:n])
				}
				if err != nil || n <= len(buf) {
					break
				}
			}
		}
	}

	return nil
}
