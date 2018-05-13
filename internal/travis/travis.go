/***
  This file is part of obscommits.

  Copyritrt (c) 2015 Peter Sztan <sztanpet@gmail.com>

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

package travis

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/sztanpet/obscommits/internal/config"
	"github.com/sztanpet/obscommits/internal/debug"
	"github.com/sztanpet/obscommits/internal/tpl"
	"github.com/sztanpet/sirc"
	"golang.org/x/net/context"
	"gopkg.in/sorcix/irc.v1"
)

type tr struct {
	cfg config.Travis
	irc *sirc.IConn
	tpl *tpl.Tpl
}

func Init(ctx context.Context) context.Context {
	cfg := config.FromContext(ctx).Travis
	tr := &tr{
		cfg: cfg,
		irc: sirc.FromContext(ctx),
		tpl: tpl.FromContext(ctx),
	}

	http.HandleFunc(tr.cfg.HookPath, tr.handler)
	return ctx
}

func (s *tr) handler(w http.ResponseWriter, r *http.Request) {
	typ := &struct {
		Type string
	}{}
	parsePayload(r, typ)
	d.D("request", r, "type:", typ.Type)

	switch typ.Type {
	case "push", "pull_request":
		s.handleType(r, typ.Type)
		return
	}

	d.D("unknown type", typ.Type)
}

func parsePayload(r *http.Request, data interface{}) error {
	if r.Header.Get("Content-Type") == "application/json" {
		dec := json.NewDecoder(r.Body)
		return dec.Decode(&data)
	}

	payload := r.FormValue("payload")
	return json.Unmarshal([]byte(payload), &data)
}

func (s *tr) handleType(r *http.Request, typ string) {
	var data struct {
		Status     string `json:"status_message"`
		Branch     string
		Message    string
		Name       string `json:"committer_name"`
		Email      string `json:"comitter_email"`
		URL        string `json:"build_url"`
		Repository struct {
			Name string
		}
	}

	err := parsePayload(r, &data)
	if err != nil {
		d.P("Error unmarshaling json:", err)
		return
	}

	pos := strings.LastIndex(data.Email, "@")
	comitter := data.Name
	if pos != -1 {
		comitter = data.Email[:pos]
	}

	pos = strings.Index(data.Message, "\n")
	message := data.Message
	if pos != -1 {
		message = data.Message[:pos]
	}
	message = strings.TrimSpace(message)

	b := bytes.NewBuffer(nil)
	s.tpl.Execute(b, "travis", &struct {
		Comitter string
		Message  string
		URL      string
		Status   string
		Repo     string
		Branch   string
	}{
		Comitter: comitter,
		Message:  message,
		URL:      data.URL,
		Status:   data.Status,
		Repo:     data.Repository.Name,
		Branch:   data.Branch,
	})

	s.writeLine(b.String())
}

func (s *tr) writeLine(line string) {
	m := &irc.Message{
		Command:  irc.PRIVMSG,
		Params:   []string{s.cfg.AnnounceChan},
		Trailing: line,
	}
	s.irc.Write(m)
}
