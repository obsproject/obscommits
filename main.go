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

	"github.com/sztanpet/obscommits/internal/analyzer"
	"github.com/sztanpet/obscommits/internal/config"
	"github.com/sztanpet/obscommits/internal/debug"
	"github.com/sztanpet/obscommits/internal/factoids"
	"github.com/sztanpet/obscommits/internal/github"
	"github.com/sztanpet/obscommits/internal/rss"
	"github.com/sztanpet/obscommits/internal/tpl"
	"golang.org/x/net/context"
)

var debuggingenabled = true

func main() {
	ctx := context.Background()
	ctx = config.Init(ctx)
	ctx = d.Init(ctx)
	ctx = tpl.Init(ctx)
	ctx = analyzer.Init(ctx)
	ctx = initIRC(ctx)
	ctx = factoids.Init(ctx)
	ctx = rss.Init(ctx)
	ctx = github.Init(ctx)

	cfg := config.FromContext(ctx).Website

	if err := http.ListenAndServe(cfg.Addr, nil); err != nil {
		d.F("ListenAndServe:", err)
	}
}
