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

package analyzer

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"sync"

	"github.com/sztanpet/obscommits/internal/config"
	"github.com/sztanpet/obscommits/internal/debug"
	"github.com/sztanpet/obscommits/internal/irc"
	"golang.org/x/net/context"
)

var (
	loglinkre  = regexp.MustCompile(`pastebin\.com/[a-zA-Z0-9]+|gist\.github\.com/(?:anonymous/)?([a-f0-9]+)`)
	analyzerre = regexp.MustCompile(`id="analyzer\-summary" data\-major\-issues="(\d+)" data\-minor\-issues="(\d+)">`)
	anurl      string
)

func Init(ctx context.Context) context.Context {
	anurl = config.FromContext(ctx).Analyzer.URL
	return ctx
}

func Handle(c *irc.IConn, m *irc.Message) (abort bool) {
	if !loglinkre.MatchString(m.Trailing) {
		return
	}

	links := loglinkre.FindAllStringSubmatch(m.Trailing, 4)
	var wg sync.WaitGroup
	linechan := make(chan string, len(links))
	query := url.Values{}
	seenlinks := make(map[string]bool)

	for _, v := range links {
		if _, ok := seenlinks[v[0]]; ok {
			continue
		}
		seenlinks[v[0]] = true
		if len(v[1]) > 0 {
			query.Set("url", "gist.github.com/anonymous/"+v[1])
		} else {
			query.Set("url", v[0])
		}
		url := anurl + query.Encode()
		wg.Add(1)
		go analyzePastebin(url, m.Prefix.Name, linechan, &wg)
	}

	go func() {
		wg.Wait()
		lines := make([]string, 0, len(seenlinks))
		for {
			select {
			case line := <-linechan:
				lines = append(lines, line)
			default:
				goto end
			}
		}
	end:
		target := c.Target(m)
		c.WriteLines(target, lines, false)
	}()

	return true
}

func analyzePastebin(url, nick string, linechan chan string, wg *sync.WaitGroup) {
	defer wg.Done()

	resp, err := http.Get(url)
	if err != nil {
		d.D("error getting link:", err)
		return
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		d.D("could not ReadAll the response body", err)
		return
	}

	issuecount := analyzerre.FindSubmatch(body)
	if len(issuecount) <= 0 {
		d.D("did not find analyzer-summary, url was", url)
		return
	}

	majorcount := string(issuecount[1])
	minorcount := string(issuecount[2])
	line := fmt.Sprintf("%s: Analyzer results [%s Major| %s Minor] %s",
		nick, majorcount, minorcount, url)
	linechan <- line
}
