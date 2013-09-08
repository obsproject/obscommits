package main

import (
	"bytes"
	rss "github.com/jteeuwen/go-pkg-rss"
	"text/template"
	"time"
)

const rssurl = "http://obsproject.com/forum/feed.php?mode=topics"

func initRSS() {
	tmpllock.Lock()
	tmpl = template.Must(tmpl.Parse(`{{define "rss"}}[Forum|{{.Author.Name}}] {{truncate .Title 150 "..."}} {{$l := index .Links 0}}{{$l.Href}}{{end}}`))
	tmpllock.Unlock()
	go pollRSS()
}

func pollRSS() {
	// 5 second timeout
	feed := rss.New(5, true, nil, itemHandler)
	for {

		if err := feed.Fetch(rssurl, nil); err != nil {
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

	if newitems[0].Id == state["lastseenrssid"] {
		return
	}

	var items []string
	tmpllock.Lock()
	for _, item := range newitems {
		if item.Id == state["lastseenrssid"] {
			break
		}

		b := bytes.NewBufferString("")
		tmpl.ExecuteTemplate(b, "rss", item)
		items = append(items, b.String())
	}
	tmpllock.Unlock()

	srv.handleLines(items)
	state["lastseenrssid"] = newitems[0].Id
	saveState()
}
