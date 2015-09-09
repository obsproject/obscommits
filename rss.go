package main

import (
	"bytes"
	"crypto/md5"
	_ "crypto/sha512"
	"crypto/tls"
	rss "github.com/jteeuwen/go-pkg-rss"
	"io/ioutil"
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/sztanpet/obscommits/internal/debug"
	"github.com/sztanpet/obscommits/internal/persist"
)

var rssurl string
var (
	messagecountre = regexp.MustCompile(`<li id="post\-\d+" class="sectionMain message`)
	mantistitlere  = regexp.MustCompile(`^\d+: (.+)`)
	forumauthorre  = regexp.MustCompile(`^.+@.+ \((.+)\)$`)
	seenLinks      = map[[16]byte]int64{}
	seenState      *persist.State
)

type sortableInt64 []int64

func (a sortableInt64) Len() int           { return len(a) }
func (a sortableInt64) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a sortableInt64) Less(i, j int) bool { return a[i] < a[j] }

func initRSS() {
	var err error
	seenState, err = persist.New("rss.state", &seenLinks)

	go pollRSS()
	go pollMantis()
}

func seenGUID(id string) (ret bool) {
	seenState.Lock()
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

	seenState.Unlock()
	seenState.Save()
	return
}

func pollMantis() {
	// 5 second timeout
	feed := rss.New(5, true, nil, mantisRSSHandler)
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	client.Transport = &http.Transport{
		TLSClientConfig:       &tls.Config{},
		ResponseHeaderTimeout: time.Second,
	}

	for {

		if err := feed.FetchClient("https://obsproject.com/mantis/issues_rss.php?", client, nil); err != nil {
			d.P("RSS fetch error:", err)
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
			d.P("RSS fetch error:", err)
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

func itemHandler(feed *rss.Feed, ch *rss.Channel, newitems []*rss.Item) {

	if len(newitems) == 0 {
		return
	}

	var items []string
	b := bytes.NewBuffer(nil)

	for _, item := range newitems {
		if seenGUID(*item.Guid) {
			continue
		}

		// we check after marking the thread as seen because regardless of the fact
		// that the check fails, we do not want to check it again
		if !checkIfThreadHasSingleMessage(*item.Guid) {
			continue
		}

		match := forumauthorre.FindStringSubmatch(item.Author.Name)
		if len(match) > 1 {
			item.Author.Name = match[1]
		}

		b.Reset()
		tmpl.execute(b, "rss", item)
		items = append(items, b.String())
	}

	go srv.handleLines("#obsproject", items, false)

}

func mantisRSSHandler(feed *rss.Feed, ch *rss.Channel, newitems []*rss.Item) {

	if len(newitems) == 0 {
		return
	}

	var items []string
	b := bytes.NewBuffer(nil)

	for _, item := range newitems {
		if seenGUID(*item.Guid) {
			continue
		}

		match := mantistitlere.FindStringSubmatch(item.Title)
		if len(match) > 1 {
			item.Title = match[1]
		}

		b.Reset()
		tmpl.execute(b, "mantisissue", item)
		items = append(items, b.String())
	}

	go srv.handleLines("#obs-dev", items, false)

}
