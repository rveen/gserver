package html

import (
	"fmt"
	"testing"

	"github.com/rveen/ogdl"
)

func Test1(t *testing.T) {

	g := ogdl.FromString("Item1\n  _link url1\nItem2\n _link url2\n")
	fmt.Println(g.Show())

	html := &Html{}

	s := html.Menu(g, "")

	fmt.Println(s)
}

func Test2(t *testing.T) {

	g := ogdl.FromString("Item1\n  _link url1\nItem2\n  Drop1 _link url2\n  Drop2 _link url3")
	fmt.Println(g.Show())

	html := &Html{}

	s := html.Menu(g)

	fmt.Println(s)
}
