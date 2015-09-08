package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"sync"

	"github.com/sztanpet/obscommits/internal/debug"
)

var (
	loglinkre  = regexp.MustCompile(`pastebin\.com/[a-zA-Z0-9]+|gist\.github\.com/(?:anonymous/)?([a-f0-9]+)`)
	analyzerre = regexp.MustCompile(`id="analyzer\-summary" data\-major\-issues="(\d+)" data\-minor\-issues="(\d+)">`)
)

func tryHandleAnalyzer(target, nick, message string) (abort bool) {
	if !loglinkre.MatchString(message) {
		return
	}

	links := loglinkre.FindAllStringSubmatch(message, 4)
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
		url := "http://obsproject.com/analyzer?" + query.Encode()
		wg.Add(1)
		go analyzePastebin(url, nick, linechan, &wg)
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
		srv.handleLines(target, lines, false)
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
