package main

import (
	"bytes"
	"html"
	"strings"
	"sync"
	"text/template"
)

type Template struct {
	t *template.Template
	sync.Mutex
}

func (t *Template) init() {
	t.Lock()
	defer t.Unlock()

	t.t = template.New("main")
	t.t.Funcs(template.FuncMap{
		"truncate": func(s string, l int, endstring string) (ret string) {
			if len(s) > l {
				ret = s[0:l-len(endstring)] + endstring
			} else {
				ret = s
			}
			return
		},
		"trim":     strings.TrimSpace,
		"unescape": html.UnescapeString,
	})

	t.t = template.Must(t.t.Parse(`{{define "git"}}[{{.Repo}}|{{.Author}}] {{truncate .Message 200 "..."}} {{.Repourl}}/commit/{{truncate .ID 7 ""}}{{end}}`))
	t.t = template.Must(t.t.Parse(`{{define "rss"}}[Forum|{{.Author.Name}}] {{truncate .Title 150 "..." | unescape}} {{$l := index .Links 0}}{{$l.Href}}{{end}}`))
	t.t = template.Must(t.t.Parse(`{{define "githubevents"}}[GH] {{.Title | unescape}} {{$l := index .Links 0}}{{$l.Href}}{{end}}`))
	t.t = template.Must(t.t.Parse(`{{define "mantisissue"}}[M|{{$c := index .Categories 0}}{{$c.Text}}] {{.Title | unescape}} {{$l := index .Links 0}}{{$l.Href}}{{end}}`))

}

func (t *Template) execute(b *bytes.Buffer, name string, data interface{}) {
	t.Lock()
	t.t.ExecuteTemplate(b, name, data)
	t.Unlock()
}
