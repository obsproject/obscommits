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

package rss

import (
	"bytes"
	"crypto/md5"
	_ "crypto/sha512"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/sztanpet/obscommits/internal/config"
	"github.com/sztanpet/obscommits/internal/debug"
	"github.com/sztanpet/obscommits/internal/persist"
	"github.com/sztanpet/obscommits/internal/tpl"
	"github.com/sztanpet/sirc"
	"golang.org/x/net/context"
	"gopkg.in/sorcix/irc.v1"
)

var (
	messagecountre = regexp.MustCompile(`<li id="post\-\d+" class="sectionMain message`)
	mantistitlere  = regexp.MustCompile(`^\d+: (.+)`)
	forumauthorre  = regexp.MustCompile(`^.+@.+ \((.+)\)$`)
	seenLinks      = map[[16]byte]int64{}
	state          *persist.State
)

type rs struct {
	cfg config.RSS
	irc *sirc.IConn
	tpl *tpl.Tpl
}

type sortableInt64 []int64

func (a sortableInt64) Len() int           { return len(a) }
func (a sortableInt64) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a sortableInt64) Less(i, j int) bool { return a[i] < a[j] }

func Init(ctx context.Context) context.Context {
	var err error
	state, err = persist.New("rss.state", &seenLinks)
	if err != nil {
		d.F(err.Error())
	}

	seenLinks = *state.Get().(*map[[16]byte]int64)

	r := &rs{
		cfg: config.FromContext(ctx).RSS,
		irc: sirc.FromContext(ctx),
		tpl: tpl.FromContext(ctx),
	}

	if len(r.cfg.ForumChan) > 0 {
		go r.pollRSS()
	}

	if len(r.cfg.MantisChan) > 0 {
		go r.pollMantis()
	}

	return ctx
}

func seenGUID(id string) (ret bool) {
	state.Lock()
	hash := md5.Sum([]byte(id))

	if _, ok := seenLinks[hash]; !ok {
		seenLinks[hash] = time.Now().UTC().UnixNano()
	} else {
		ret = true
	}

	if len(seenLinks) > 2000 {
		timestamps := make(sortableInt64, 0, len(seenLinks))
		for _, ts := range seenLinks {
			timestamps = append(timestamps, ts)
		}

		sort.Sort(timestamps)
		timestamps = timestamps[:len(seenLinks)-1500]
		for key, value := range seenLinks {

			for _, ts := range timestamps {
				if value == ts {
					delete(seenLinks, key)
					break
				}
			}

		}
	}

	state.Unlock()
	state.Save()
	return
}

func (r *rs) pollMantis() {
	fp := gofeed.NewParser()

	for {
		feed, err := fp.ParseURL(r.cfg.MantisURL)
		if err != nil {
			d.P("RSS fetch error:", err)
			<-time.After(5 * time.Minute)
			continue
		}

		r.mantisRSSHandler(feed)
		<-time.After(5 * time.Minute)
	}
}

func (r *rs) pollRSS() {
	fp := gofeed.NewParser()

	for {
		feed, err := fp.ParseURL(r.cfg.ForumURL)
		if err != nil {
			d.P("RSS fetch error:", err)
			<-time.After(5 * time.Minute)
			continue
		}

		r.itemHandler(feed)
		<-time.After(5 * time.Minute)
	}
}

func checkIfThreadHasSingleMessage(link string) bool {
	hassinglemessage := make(chan bool)

	go (func() {
		client := &http.Client{
			Timeout: 2 * time.Second,
			Transport: &http.Transport{
				ResponseHeaderTimeout: time.Second,
			},
		}
		resp, err := client.Get(link)
		ret := true // to be safe, we default to true meaning announce the topic
		defer (func() {
			hassinglemessage <- ret
		})()

		if err != nil {
			d.D("Could not get link", link, err)
			return
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			d.D("Error reading from the body of the link", link, err)
			return
		}

		count := messagecountre.FindAllIndex(body, 2)
		if len(count) == 0 {
			d.D("Did not find the messagecountre regex in the link", link)
		} else if len(count) > 1 {
			ret = false
		}
	})()

	return <-hassinglemessage
}

func (r *rs) itemHandler(feed *gofeed.Feed) {

	if len(feed.Items) == 0 {
		return
	}

	var items []string
	b := bytes.NewBuffer(nil)

	for _, item := range feed.Items {
		if seenGUID(item.GUID) {
			continue
		}

		// we check after marking the thread as seen because regardless of the fact
		// that the check fails, we do not want to check it again
		if !checkIfThreadHasSingleMessage(item.GUID) {
			continue
		}

		match := forumauthorre.FindStringSubmatch(item.Author.Name)
		if len(match) > 1 {
			item.Author.Name = match[1]
		}

		b.Reset()
		r.tpl.Execute(b, "rss", item)
		items = append(items, b.String())
	}

	go r.writeLines(r.cfg.ForumChan, items)
}

func (r *rs) mantisRSSHandler(feed *gofeed.Feed) {

	if len(feed.Items) == 0 {
		return
	}

	var items []string
	b := bytes.NewBuffer(nil)

	for _, item := range feed.Items {
		if seenGUID(item.GUID) {
			continue
		}

		match := mantistitlere.FindStringSubmatch(item.Title)
		if len(match) > 1 {
			item.Title = match[1]
		}

		b.Reset()
		r.tpl.Execute(b, "mantisissue", item)
		items = append(items, b.String())
	}

	go r.writeLines(r.cfg.MantisChan, items)
}

func (r *rs) writeLines(ch string, lines []string) {
	l := len(lines)

	if l == 0 {
		return
	}

	if l > 5 {
		lines = lines[:5]
	}

	for _, l := range lines {
		r.irc.Write(&irc.Message{
			Command:  irc.PRIVMSG,
			Params:   []string{ch},
			Trailing: l,
		})
	}
}
