package main

import (
	"bytes"
	_ "crypto/sha512"
	"crypto/tls"
	rss "github.com/jteeuwen/go-pkg-rss"
	"io/ioutil"
	"net/http"
	"regexp"
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
	go pollRSS()
	go pollGitHub()
}

func pollGitHub() {
	// 5 second timeout
	feed := rss.New(5, true, nil, githubRSSHandler)
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	if len(githubnewsurl) > 8 && githubnewsurl[:8] == "https://" {
		client.Transport = &http.Transport{
			TLSClientConfig:       &tls.Config{},
			ResponseHeaderTimeout: time.Second,
		}
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
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	if len(rssurl) > 8 && rssurl[:8] == "https://" {
		client.Transport = &http.Transport{
			TLSClientConfig:       &tls.Config{},
			ResponseHeaderTimeout: time.Second,
		}
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

	var items []string
	b := bytes.NewBuffer(nil)

	for _, item := range newitems {
		if !state.addRssHash(*item.Guid) {
			continue
		}

		// we check after marking the thread as seen because regardless of the fact
		// that the check fails, we do not want to check it again
		if !checkIfThreadHasSingleMessage(*item.Guid) {
			continue
		}

		b.Reset()
		tmpl.execute(b, "rss", item)
		items = append(items, b.String())
	}

	go srv.handleLines("#obsproject", items, false)

}

func githubRSSHandler(feed *rss.Feed, ch *rss.Channel, newitems []*rss.Item) {

	if len(newitems) == 0 {
		return
	}

	var items []string
	b := bytes.NewBuffer(nil)

	for _, item := range newitems {
		if !githubeventsre.MatchString(item.Title) ||
			!githublinkre.MatchString((*item.Links[0]).Href) {
			continue
		}

		if !state.addGithubEvent(item.Id) {
			continue
		}

		b.Reset()
		tmpl.execute(b, "githubevents", item)
		items = append(items, b.String())
	}

	go srv.handleLines("#obs-dev", items, false)

}
