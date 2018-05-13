/***
  This file is part of obscommits.

  Copyright (c) 2015 Peter Sztan <sztanpet@gmail.com>

  obscommits is free software; you can redistribute it and/or modify it
  under the terms of the GNU Lesser General Public License as published by
  the Free Software Foundation; either version 3 of the License, or
  (at your option) any later version.

  obscommits is distributed in the hope that it will be useful, but
  WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
  Lesser General Public License for more details.

  You should have received a copy of the GNU Lesser General Public License
  along with obscommits; If not, see <http://www.gnu.org/licenses/>.
***/

package tpl

import (
	"bytes"
	"html"
	"strings"
	"sync"
	"text/template"

	"golang.org/x/net/context"
)

type Tpl struct {
	t *template.Template
	sync.Mutex
}

const tplStr = `
{{define "push"}}[{{.Repo}}|{{.Author}}] {{truncate .Message 200 "..."}} {{.RepoURL}}/commit/{{truncate .ID 7 ""}}{{end}}
{{define "pushSkipped"}}[{{.Repo}}|{{.Author}}] Skipping announcement of {{.SkipCount}} commits: {{.RepoURL}}/compare/{{truncate .FromID 7 ""}}...{{truncate .ToID 7 ""}}{{end}}
{{define "pr"}}[GH PR|{{.Author}}] {{.Title | unescape}} {{.URL | unescape}}{{end}}
{{define "wiki"}}[GH Wiki|{{.Author}}] {{.Page | unescape}} {{.Action}} {{.URL | unescape}}{{if ne .Action "created"}}/_compare/{{truncate .Sha 7 ""}}%5E...{{truncate .Sha 7 ""}}{{end}}{{end}}
{{define "issues"}}[GH Issue|{{.Author}}] {{.Title | unescape}} {{.URL | unescape}}{{end}}
{{define "rss"}}[Forum|{{.Author.Name}}] {{truncate .Title 150 "..." | unescape}} {{.Link}}{{end}}
{{define "mantisissue"}}[M|{{$c := index .Categories 0}}{{$c}}] {{.Title | unescape}} {{.Link}}{{end}}
{{define "travis"}}{{$needBold := eq .Status "Passed" "Fixed"}}[CI|{{if $needBold}}{{end}}{{.Status}}{{if $needBold}}{{end}}] {{.Repo}}/{{.Branch}} ({{.Comitter}} - {{truncate .Message 200 "..."}}) {{.URL}}{{end}}
`

func Init(ctx context.Context) context.Context {
	t := &Tpl{}
	t.init()

	return context.WithValue(ctx, "tpl", t)
}

func FromContext(ctx context.Context) *Tpl {
	return ctx.Value("tpl").(*Tpl)
}

func (t *Tpl) init() {
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

	t.t = template.Must(t.t.Parse(tplStr))
}

func (t *Tpl) Execute(b *bytes.Buffer, name string, data interface{}) {
	t.Lock()
	t.t.ExecuteTemplate(b, name, data)
	t.Unlock()
}
