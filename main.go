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

package main

import (
	"net/http"

	"github.com/sztanpet/obscommits/internal/config"
	"github.com/sztanpet/obscommits/internal/debug"
	"golang.org/x/net/context"
)

var debuggingenabled = true
var state State
var tmpl Template

func main() {
	ctx := context.Background()
	ctx = config.Init(ctx)
	ctx = d.Init(ctx)

	cfg := config.GetFromContext(ctx)
	debuggingenabled = cfg.Debug.Debug
	ircaddr := cfg.IRC.Addr
	listenaddr := cfg.Website.Addr
	githookpath := cfg.Github.HookPath
	factoidhookpath := cfg.Factoids.HookPath
	rssurl = cfg.RSS.ForumURL

	state.init()
	tmpl.init()
	initIRC(ircaddr)

	initRSS()
	initFactoids(factoidhookpath)
	initGithub(githookpath)

	if err := http.ListenAndServe(listenaddr, nil); err != nil {
		F("ListenAndServe:", err)
	}
}
