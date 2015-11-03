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

package github

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sztanpet/obscommits/internal/config"
	"github.com/sztanpet/obscommits/internal/debug"
	"github.com/sztanpet/obscommits/internal/irc"
	"github.com/sztanpet/obscommits/internal/tpl"
	"golang.org/x/net/context"
)

const maxLines = 5

var (
	srv  *irc.IConn
	tmpl *tpl.Tpl
	cfg  config.Github
)

func Init(ctx context.Context) context.Context {
	srv = irc.FromContext(ctx)
	tmpl = tpl.FromContext(ctx)
	cfg = config.FromContext(ctx).Github

	http.HandleFunc(cfg.HookPath, func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("X-Github-Event") {
		case "push":
			pushHandler(r)
		case "gollum":
			wikiHandler(r)
		case "pull_request":
			prHandler(r)
		case "issues":
			issueHandler(r)
		}
	})

	return ctx
}

func handlePayload(r *http.Request, data interface{}) error {
	if r.Header.Get("Content-Type") == "application/json" {
		dec := json.NewDecoder(r.Body)
		return dec.Decode(&data)
	} else {
		payload := r.FormValue("payload")
		return json.Unmarshal([]byte(payload), &data)
	}
}

func pushHandler(r *http.Request) {
	var data struct {
		Ref     string
		Before  string
		Commits []struct {
			Author struct {
				Username string
			}
			URL     string
			Message string
			Id      string
		}
		Repository struct {
			Name string
			URL  string
		}
	}

	err := handlePayload(r, &data)
	if err != nil {
		d.P("Error unmarshaling json:", err)
		return
	}

	pos := strings.LastIndex(data.Ref, "/") + 1
	branch := data.Ref[pos:]
	lines := make([]string, 0, 5)
	repo := data.Repository.Name
	repoURL := data.Repository.URL
	b := bytes.NewBuffer(nil)

	if branch != "master" {
		return
	}

	// if we want to print more than 5 lines, just print two lines, one line
	// announcing that commits are skipped with a compare view of the commits
	// and the last line, usually a merge commit
	needSkip := len(data.Commits) > maxLines

	for k, v := range data.Commits {
		firstline := strings.TrimSpace(v.Message)
		pos = strings.Index(firstline, "\n")
		if pos > 0 {
			firstline = strings.TrimSpace(firstline[:pos])
		}

		b.Reset()
		if needSkip && k == len(data.Commits)-2 {
			tmpl.Execute(b, "pushSkipped", &struct {
				Author    string // commits[i].author.username
				FromID    string // .before
				ToID      string // commits[len - 2].id
				SkipCount int
				Repo      string // repository.name
				RepoURL   string // repository.URL
			}{
				Author:    v.Author.Username,
				FromID:    data.Before,
				ToID:      v.Id,
				SkipCount: len(data.Commits) - 1,
				Repo:      repo,
				RepoURL:   repoURL,
			})
		} else if !needSkip || k > len(data.Commits)-2 {
			tmpl.Execute(b, "push", &struct {
				Author  string // commits[i].author.username
				URL     string // commits[i].URL
				Message string // commits[i].message
				ID      string // commits[i].id
				Repo    string // repository.name
				RepoURL string // repository.URL
				Branch  string // .ref the part after refs/heads/
			}{
				Author:  v.Author.Username,
				URL:     v.URL,
				Message: firstline,
				ID:      v.Id,
				Repo:    repo,
				RepoURL: repoURL,
				Branch:  branch,
			})
		}

		if b.Len() > 0 {
			lines = append(lines, b.String())
		}
	}

	go srv.WriteLines(cfg.AnnounceChan, lines, true)
}

func prHandler(r *http.Request) {
	var data struct {
		Action       string
		Pull_request struct {
			Html_URL string
			Title    string
			User     struct {
				Login string
			}
		}
	}

	err := handlePayload(r, &data)
	if err != nil {
		d.P("Error unmarshaling json:", err)
		return
	}

	if data.Action != "opened" {
		return
	}

	b := bytes.NewBuffer(nil)
	tmpl.Execute(b, "pr", &struct {
		Author string
		Title  string
		URL    string
	}{
		Author: data.Pull_request.User.Login,
		Title:  data.Pull_request.Title,
		URL:    data.Pull_request.Html_URL,
	})

	go srv.WriteLines(cfg.AnnounceChan, []string{b.String()}, true)
}

func wikiHandler(r *http.Request) {
	var data struct {
		Pages []struct {
			Page_name string
			Action    string
			Sha       string
			Html_URL  string
		}
		Sender struct {
			Login string
		}
	}

	err := handlePayload(r, &data)
	if err != nil {
		d.P("Error unmarshaling json:", err)
		return
	}

	lines := make([]string, 0, len(data.Pages))
	b := bytes.NewBuffer(nil)
	for _, v := range data.Pages {

		b.Reset()
		tmpl.Execute(b, "wiki", &struct {
			Author string
			Page   string
			URL    string
			Action string
			Sha    string
		}{
			Author: data.Sender.Login,
			Page:   v.Page_name,
			URL:    v.Html_URL,
			Action: v.Action,
			Sha:    v.Sha,
		})
		lines = append(lines, b.String())
	}

	if l := len(lines); l > 5 {
		lines = lines[l-5:]
	}

	go srv.WriteLines(cfg.AnnounceChan, lines, true)
}

func issueHandler(r *http.Request) {
	var data struct {
		Action string
		Issue  struct {
			Title    string
			Html_URL string
			User     struct {
				Login string
			}
		}
	}

	err := handlePayload(r, &data)
	if err != nil {
		d.P("Error unmarshaling json:", err)
		return
	}

	if data.Action != "opened" {
		return
	}

	b := bytes.NewBuffer(nil)
	tmpl.Execute(b, "issues", &struct {
		Author string
		Title  string
		URL    string
	}{
		Author: data.Issue.User.Login,
		Title:  data.Issue.Title,
		URL:    data.Issue.Html_URL,
	})

	go srv.WriteLines(cfg.AnnounceChan, []string{b.String()}, true)
}
