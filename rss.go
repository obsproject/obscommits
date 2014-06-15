package main

import (
	"bytes"
	_ "crypto/sha512"
	"crypto/tls"
	rss "github.com/jteeuwen/go-pkg-rss"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"text/template"
	"time"
)

var rssurl string
var githubnewsurl string
var (
	messagecountre = regexp.MustCompile(`<li id="post\-\d+" class="sectionMain message`)
	githubeventsre = regexp.MustCompile(`.* opened (issue|pull request) .*`)
	githublinkre   = regexp.MustCompile(`.*//github.com/jp9000/.*`)
)

func initRSS() {
	tmpllock.Lock()
	tmpl = template.Must(tmpl.Parse(`{{define "rss"}}[Forum|{{.Author.Name}}] {{truncate .Title 150 "..." | unescape}} {{$l := index .Links 0}}{{$l.Href}}{{end}}`))
	tmpl = template.Must(tmpl.Parse(`{{define "githubevents"}}[GH] {{.Title | unescape}} {{$l := index .Links 0}}{{$l.Href}}{{end}}`))
	tmpllock.Unlock()
	go pollRSS()
	go pollGitHub()
}

func pollGitHub() {
	// 5 second timeout
	feed := rss.New(5, true, nil, githubRSSHandler)
	client := http.DefaultClient
	if len(githubnewsurl) > 8 && githubnewsurl[:8] == "https://" {
		client = &http.Client{Transport: &http.Transport{
			TLSClientConfig:       &tls.Config{},
			ResponseHeaderTimeout: time.Second,
		}}
	}

	for {

		if err := feed.FetchClient(githubnewsurl, client, nil); err != nil {
			P("RSS fetch error:", err)
			<-time.After(5 * time.Minute)
			continue
		}

		<-time.After(time.Duration(feed.SecondsTillUpdate() * int64(time.Second)))
	}
}

func pollRSS() {
	// 5 second timeout
	feed := rss.New(5, true, nil, itemHandler)
	client := http.DefaultClient
	if len(rssurl) > 8 && rssurl[:8] == "https://" {
		client = &http.Client{Transport: &http.Transport{
			TLSClientConfig:       &tls.Config{},
			ResponseHeaderTimeout: time.Second,
		}}
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

func checkIfThreadHasSingleMessage(link string) bool {
	hassinglemessage := make(chan bool)

	go (func() {
		client := &http.Client{Transport: &http.Transport{
			ResponseHeaderTimeout: time.Second,
		}}
		resp, err := client.Get(link)
		ret := true // to be safe, we default to true meaning announce the topic
		defer (func() {
			hassinglemessage <- ret
		})()

		if err != nil {
			D("Could not get link", link, err)
			return
		}

		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			D("Error reading from the body of the link", link, err)
			return
		}

		count := messagecountre.FindAllIndex(body, 2)
		if len(count) == 0 {
			D("Did not find the messagecountre regex in the link", link)
		} else if len(count) > 1 {
			ret = false
		}
	})()

	return <-hassinglemessage
}

func itemHandler(feed *rss.Feed, ch *rss.Channel, newitems []*rss.Item) {

	if len(newitems) == 0 {
		return
	}

	statelock.Lock()
	defer statelock.Unlock()

	var items []string
	tmpllock.Lock()
	b := bytes.NewBuffer(nil)

	for _, item := range newitems {
		hash := getHash(*item.Guid)
		if _, ok := state.Seenrss[hash]; ok {
			continue
		}
		state.Seenrss[hash] = time.Now().UTC().UnixNano()
		// we check after marking the thread as seen because regardless of the fact
		// that the check fails, we do not want to check it again
		if !checkIfThreadHasSingleMessage(*item.Guid) {
			continue
		}

		b.Reset()
		tmpl.ExecuteTemplate(b, "rss", item)
		items = append(items, b.String())
	}
	tmpllock.Unlock()

	go srv.handleLines("#obsproject", items, false)

	if len(state.Seenrss) > 2000 { // GC old items, sort them by time, delete all that have the value beyond the last 2000
		rsstimestamps := make(sortableInt64, 0, len(state.Seenrss))
		for _, ts := range state.Seenrss {
			rsstimestamps = append(rsstimestamps, ts)
		}
		sort.Sort(rsstimestamps)
		rsstimestamps = rsstimestamps[:len(state.Seenrss)-2000]
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

func githubRSSHandler(feed *rss.Feed, ch *rss.Channel, newitems []*rss.Item) {

	if len(newitems) == 0 {
		return
	}

	statelock.Lock()
	defer statelock.Unlock()

	var items []string
	tmpllock.Lock()
	b := bytes.NewBuffer(nil)

	for _, item := range newitems {
		if !githubeventsre.MatchString(item.Title) || !githublinkre.MatchString((*item.Links[0]).Href) {
			continue
		}

		hash := getHash(item.Id)
		if _, ok := state.Seengithubevents[hash]; ok {
			continue
		}
		state.Seengithubevents[hash] = time.Now().UTC().UnixNano()

		b.Reset()
		tmpl.ExecuteTemplate(b, "githubevents", item)
		items = append(items, b.String())
	}
	tmpllock.Unlock()

	go srv.handleLines("#obs-dev", items, false)

	if len(state.Seengithubevents) > 30 {
		rsstimestamps := make(sortableInt64, 0, len(state.Seengithubevents))
		for _, ts := range state.Seengithubevents {
			rsstimestamps = append(rsstimestamps, ts)
		}
		sort.Sort(rsstimestamps)
		rsstimestamps = rsstimestamps[:len(state.Seengithubevents)-30]
		for key, value := range state.Seengithubevents {
			for _, ts := range rsstimestamps {
				if value == ts {
					delete(state.Seengithubevents, key)
				}
			}
		}
	}

	saveState()
}
