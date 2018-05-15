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

	"github.com/obsproject/obscommits/internal/config"
	"github.com/obsproject/obscommits/internal/debug"
	"github.com/obsproject/obscommits/internal/tpl"
	"github.com/sztanpet/sirc"
	"golang.org/x/net/context"
	"gopkg.in/sorcix/irc.v1"
)

const maxLines = 5

type gh struct {
	cfg config.Github
	irc *sirc.IConn
	tpl *tpl.Tpl
}

func Init(ctx context.Context) context.Context {
	cfg := config.FromContext(ctx).Github
	gh := &gh{
		cfg: cfg,
		irc: sirc.FromContext(ctx),
		tpl: tpl.FromContext(ctx),
	}

	http.HandleFunc(gh.cfg.HookPath, gh.handler)
	return ctx
}

func (s *gh) handler(w http.ResponseWriter, r *http.Request) {
	d.D("request", r)
	switch r.Header.Get("X-Github-Event") {
	case "push":
		s.pushHandler(r)
	case "gollum":
		s.wikiHandler(r)
	case "pull_request":
		s.prHandler(r)
	case "issues":
		s.issueHandler(r)
	}
}

func handlePayload(r *http.Request, data interface{}) error {
	if r.Header.Get("Content-Type") == "application/json" {
		dec := json.NewDecoder(r.Body)
		return dec.Decode(&data)
	}

	payload := r.FormValue("payload")
	return json.Unmarshal([]byte(payload), &data)
}

func (s *gh) pushHandler(r *http.Request) {
	var data struct {
		Ref     string
		Before  string
		Commits []struct {
			Author struct {
				Username string
			}
			URL     string
			Message string
			ID      string
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
	lines := make([]string, 0, maxLines)
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
			s.tpl.Execute(b, "pushSkipped", &struct {
				Author    string // commits[i].author.username
				FromID    string // commits[0].id
				ToID      string // commits[len - 2].id
				SkipCount int
				Repo      string // repository.name
				RepoURL   string // repository.url
			}{
				Author:    v.Author.Username,
				FromID:    data.Before,
				ToID:      v.ID,
				SkipCount: len(data.Commits) - 1,
				Repo:      repo,
				RepoURL:   repoURL,
			})
		} else if !needSkip || k > len(data.Commits)-2 {
			s.tpl.Execute(b, "push", &struct {
				Author  string // commits[i].author.username
				URL     string // commits[i].url
				Message string // commits[i].message
				ID      string // commits[i].id
				Repo    string // repository.name
				RepoURL string // repository.url
				Branch  string // .ref the part after refs/heads/
			}{
				Author:  v.Author.Username,
				URL:     v.URL,
				Message: firstline,
				ID:      v.ID,
				Repo:    repo,
				RepoURL: repoURL,
				Branch:  branch,
			})
		}

		if b.Len() > 0 {
			lines = append(lines, b.String())
		}
	}

	for _, line := range lines {
		s.writeLine(line)
	}
}

func (s *gh) prHandler(r *http.Request) {
	var data struct {
		Action string
		PR     struct {
			URL   string `json:"html_url"`
			Title string
			User  struct {
				Login string
			}
		} `json:"pull_request"`
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
	s.tpl.Execute(b, "pr", &struct {
		Author string
		Title  string
		URL    string
	}{
		Author: data.PR.User.Login,
		Title:  data.PR.Title,
		URL:    data.PR.URL,
	})

	s.writeLine(b.String())
}

func (s *gh) wikiHandler(r *http.Request) {
	var data struct {
		Pages []struct {
			Page   string `json:"page_name"`
			Action string
			Sha    string
			URL    string `json:"html_url"`
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
		s.tpl.Execute(b, "wiki", &struct {
			Author string
			Page   string
			URL    string
			Action string
			Sha    string
		}{
			Author: data.Sender.Login,
			Page:   v.Page,
			URL:    v.URL,
			Action: v.Action,
			Sha:    v.Sha,
		})
		lines = append(lines, b.String())
	}

	if l := len(lines); l > maxLines {
		lines = lines[l-maxLines:]
	}

	for _, line := range lines {
		s.writeLine(line)
	}
}

func (s *gh) issueHandler(r *http.Request) {
	var data struct {
		Action string
		Issue  struct {
			Title string
			URL   string `json:"html_url"`
			User  struct {
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
	s.tpl.Execute(b, "issues", &struct {
		Author string
		Title  string
		URL    string
	}{
		Author: data.Issue.User.Login,
		Title:  data.Issue.Title,
		URL:    data.Issue.URL,
	})

	s.writeLine(b.String())
}

func (s *gh) writeLine(line string) {
	m := &irc.Message{
		Command:  irc.PRIVMSG,
		Params:   []string{s.cfg.AnnounceChan},
		Trailing: line,
	}
	s.irc.Write(m)
}
