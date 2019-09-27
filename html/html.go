package html

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/rveen/ogdl"
)

type Html struct{}

func (h Html) IsImage(p string) bool {
	p = strings.ToLower(p)
	if strings.HasSuffix(p, ".jpg") || strings.HasSuffix(p, ".png") {
		return true
	}
	return false
}

// TODO move this elsewhere
func (h Html) RepoBase(path, base string) string {

	if strings.HasPrefix(path, base) {
		return path[len(base):]
	}
	return path
}

func (h Html) Menu(g *ogdl.Graph, pre string) string {

	var buf bytes.Buffer

	for _, n := range g.Out {

		link := n.Node("_link")
		if link != nil {
			// Top link
			// <li class="nav-item"><a class="nav-link" href="$_link">$name</a></li>
			buf.WriteString("<li class='nav-item'><a class='nav-link' href='")
			path := filepath.Clean(link.String())
			if path[0] == '/' {
				path = path[1:]
			}
			buf.WriteString(pre + path + "'>" + n.ThisString() + "</a></li>\n")
		} else {
			// Top dropdown
			// <li class="nav-item dropdown">
			//        <a href="#" class="dropdown-toggle nav-link" type="button" data-toggle="dropdown" aria-haspopup="true" aria-expanded="false">
			//          Dropdown
			//        </a>
			// <ul class="dropdown-menu multi-level" role="menu" aria-labelledby="dropdownMenu">
			buf.WriteString("<li class='nav-item dropdown'><a href='#' class='dropdown-toggle nav-link' data-toggle='dropdown' aria-haspopup='true' aria-expanded='false'>\n")
			buf.WriteString(n.ThisString())
			buf.WriteString("</a>\n<ul class='dropdown-menu multi-level' role='menu' aria-labelledby='dropdownMenu'>\n")

			for _, d := range n.Out {
				link = d.Node("_link")
				if link != nil {
					// Dropdown menu link
					// <li class="dropdown-item"><a href="$_link">$name</a></li>
					buf.WriteString("<li class='dropdown-item'><a href='")
					path := filepath.Clean(link.String())
					if path[0] == '/' {
						path = path[1:]
					}
					buf.WriteString(pre + path + "'>" + d.ThisString() + "</a></li>\n")
				} else {
					submenu(d, buf, pre)
				}

			}
			buf.WriteString("</ul></li>\n")
		}
	}

	// If the entry has subentries, use a dropdown. Further subentries use dropdown-submenu

	// Top level *menu*, start:
	// <li class="nav-item dropdown">
	//        <a href="#" class="dropdown-toggle nav-link" type="button" data-toggle="dropdown" aria-haspopup="true" aria-expanded="false">
	//          Dropdown
	//        </a>
	// <ul class="dropdown-menu multi-level" role="menu" aria-labelledby="dropdownMenu">

	// Menu link
	// <li class="dropdown-item"><a href="$_link">$name</a></li>

	// Submenu init
	// <li class="dropdown-submenu">
	// <a  class="dropdown-item" tabindex="-1" href="#">$name</a>
	// <ul class="dropdown-menu">

	// Subitem link
	// <li class="dropdown-item"><a tabindex="-1" href="$_link">$name</a></li>

	// Submenu end
	// </ul>

	// END: </ul></li>

	return buf.String()

	/* <li class="nav-item dropdown">
	    <a href="#" class="dropdown-toggle nav-link" type="button" id="dropdownMenu1" data-toggle="dropdown" aria-haspopup="true" aria-expanded="false">
	      Dropdown
	    </a>
	    <ul class="dropdown-menu multi-level" role="menu" aria-labelledby="dropdownMenu">
	        <li class="dropdown-item"><a href="#">Some action</a></li>
	        <li class="dropdown-item"><a href="#">Some other action</a></li>
	        <li class="dropdown-divider"></li>
	        <li class="dropdown-submenu">
	          <a  class="dropdown-item" tabindex="-1" href="#">Hover me for more options</a>
	          <ul class="dropdown-menu">
	            <li class="dropdown-item"><a tabindex="-1" href="#">Second level</a></li>
	            <li class="dropdown-submenu">
	              <a class="dropdown-item" href="#">Even More..</a>
	              <ul class="dropdown-menu">
	                  <li class="dropdown-item"><a href="#">3rd level</a></li>
	                    <li class="dropdown-submenu"><a class="dropdown-item" href="#">another level</a>
	                    <ul class="dropdown-menu">
	                        <li class="dropdown-item"><a href="#">4th level</a></li>
	                        <li class="dropdown-item"><a href="#">4th level</a></li>
	                        <li class="dropdown-item"><a href="#">4th level</a></li>
	                    </ul>
	                  </li>
	                    <li class="dropdown-item"><a href="#">3rd level</a></li>
	              </ul>
	            </li>
	            <li class="dropdown-item"><a href="#">Second level</a></li>
	            <li class="dropdown-item"><a href="#">Second level</a></li>
	          </ul>
	        </li>
	      </ul>
	</li> */
}

func submenu(g *ogdl.Graph, buf bytes.Buffer, pre string) {

	// Submenu init
	// <li class="dropdown-submenu">
	// <a  class="dropdown-item" tabindex="-1" href="#">$name</a>
	// <ul class="dropdown-menu">
	buf.WriteString("<li class='dropdown-submenu'>")
	buf.WriteString("<a  class='dropdown-item' tabindex='-1' href='#'>")
	buf.WriteString(g.ThisString())
	buf.WriteString("</a><ul class='dropdown-menu'>\n")

	// Subitem link
	// <li class="dropdown-item"><a tabindex="-1" href="$_link">$name</a></li>

	for _, n := range g.Out {
		link := n.Node("_link")
		if link != nil {
			// Dropdown menu link
			// <li class="dropdown-item"><a href="$_link">$name</a></li>
			buf.WriteString("<li class='dropdown-item'><a tabindex='-1' href='")
			buf.WriteString(pre + link.String() + "'>" + n.ThisString() + "</a></li>\n")
		} else {
			submenu(n, buf, pre)
		}
	}

	// Submenu end
	buf.WriteString("</ul>\n")

}

func (h Html) ChecklistHighlight(s string) string {

	s = strings.ReplaceAll(s, "<li>☐", "<li class='checklist'><span style='color: orange'>☐</span>")
	s = strings.ReplaceAll(s, "<li>☒", "<li class='checklist'><span style='color: red'>☒</span>")
	s = strings.ReplaceAll(s, "<li>☑", "<li class='checklist'><span style='color: green'>☑</span>")

	s = strings.ReplaceAll(s, "[!]", "<span style='color: orange'>⚠</span>")

	return s
}

func (h Html) ChecklistDone(s string) int {
	return strings.Count(s, "☑")
}

func (h Html) ChecklistPending(s string) int {

	return strings.Count(s, "☐")
}

func (h Html) Breadcrumb(url string) string {

	println("breadcrumb ", url)

	r := "<span class='bc_sep1'>/</span>"
	p := ""

	ss := strings.Split(url, "/")
	for i, s := range ss {
		if s == "" {
			continue
		}

		p += "/" + s
		if i < len(ss)-1 {
			r += "<a href='" + p + "'>" + s + "</a><span class='bc_sep'>/</span>"
		} else {
			r += s
		}

	}

	return r
}
