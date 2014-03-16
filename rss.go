package main

import (
	"bytes"
	"crypto/tls"
	rss "github.com/jteeuwen/go-pkg-rss"
	"net/http"
	"sort"
	"text/template"
	"time"
)

var rssurl string

func initRSS() {
	tmpllock.Lock()
	tmpl = template.Must(tmpl.Parse(`{{define "rss"}}[Forum|{{.Author.Name}}] {{truncate .Title 150 "..." | unescape}} {{$l := index .Links 0}}{{$l.Href}}{{end}}`))
	tmpllock.Unlock()
	go pollRSS()
}

func pollRSS() {
	// 5 second timeout
	feed := rss.New(5, true, nil, itemHandler)
	client := http.DefaultClient
	if len(rssurl) > 8 && rssurl[:8] == "https://" {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{},
		}
		client = &http.Client{Transport: tr}
	}

	for {

		if err := feed.FetchClient(rssurl, client, nil); err != nil {
			P("RSS fetch error:", err)
			<-time.After(5 * time.Minute)
			continue
		}

		<-time.After(time.Duration(feed.SecondsTillUpdate() * int64(time.Second)))
	}
}

func itemHandler(feed *rss.Feed, ch *rss.Channel, newitems []*rss.Item) {

	if len(newitems) == 0 {
		return
	}

	statelock.Lock()
	defer statelock.Unlock()

	firsthash := getHash(*newitems[0].Guid)
	if _, ok := state.Seenrss[firsthash]; ok {
		return // already seen
	}

	var items []string
	tmpllock.Lock()
	for _, item := range newitems {
		hash := getHash(*item.Guid)
		if _, ok := state.Seenrss[hash]; ok {
			break
		}
		state.Seenrss[hash] = time.Now().UTC().UnixNano()
		b := bytes.NewBufferString("")
		tmpl.ExecuteTemplate(b, "rss", item)
		items = append(items, b.String())
	}
	tmpllock.Unlock()

	go srv.handleLines(items, false, false)

	if len(state.Seenrss) > 400 { // GC old items, sort them by time, delete all that have the value beyond the last 400
		rsstimestamps := make(sortableInt64, 0, len(state.Seenrss))
		for _, ts := range state.Seenrss {
			rsstimestamps = append(rsstimestamps, ts)
		}
		sort.Sort(rsstimestamps)
		rsstimestamps = rsstimestamps[:len(state.Seenrss)-400]
		for key, value := range state.Seenrss {
			for _, ts := range rsstimestamps {
				if value == ts {
					delete(state.Seenrss, key)
				}
			}
		}
	}

	saveState()
}
