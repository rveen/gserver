package files

import (
	"log"
	"testing"
)

func Test1(t *testing.T) {

	f := Dir("/files/eclipse/go/src/gserver/app/web")

	fe := &FileEntry{}
	s, v, err := f.Get("index.htm", fe)

	log.Println(v, s, err)

}

func Test2(t *testing.T) {

	f := Dir("/files/eclipse/go/src/gserver/app/web")

	fe := &FileEntry{}
	s, v, err := f.Get("pepe/index.htm", fe)

	log.Println(v, s, err)

}
