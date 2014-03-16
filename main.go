package main

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/gob"
	conf "github.com/msbranco/goconfig"
	"html"
	"io/ioutil"
	"strings"
	"sync"
	"text/template"
)

type sortableInt64 []int64

func (a sortableInt64) Len() int           { return len(a) }
func (a sortableInt64) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a sortableInt64) Less(i, j int) bool { return a[i] < a[j] }

type State struct {
	Factoids map[string]string
	Seenrss  map[string]int64
	Admins   map[string]bool
}

var debuggingenabled = true
var state State
var (
	tmpl      *template.Template
	tmpllock  = sync.RWMutex{}
	statelock = sync.RWMutex{}
)

func main() {
	c, err := conf.ReadConfigFile("settings.cfg")
	if err != nil {
		nc := conf.NewConfigFile()
		nc.AddOption("default", "debug", "false")
		nc.AddOption("default", "ircserver", "irc.quakenet.org:6667")
		nc.AddOption("default", "listenaddress", ":9998")
		nc.AddSection("git")
		nc.AddOption("git", "hookpath", "/whatever")
		nc.AddSection("rss")
		nc.AddOption("rss", "url", "https://obsproject.com/forum/list/-/index.rss")

		if err := nc.WriteConfigFile("settings.cfg", 0644, "OBScommits settings file"); err != nil {
			F("Unable to create settings.cfg: ", err)
		}
		if c, err = conf.ReadConfigFile("settings.cfg"); err != nil {
			F("Unable to read settings.cfg: ", err)
		}
	}

	debuggingenabled, _ = c.GetBool("default", "debug")
	ircaddr, _ := c.GetString("default", "ircserver")
	listenaddr, _ := c.GetString("default", "listenaddress")
	hookpath, _ := c.GetString("git", "hookpath")
	rssurl, _ = c.GetString("rss", "url")

	loadState()
	initTemplate()
	initIRC(ircaddr)
	initRSS()
	initGithub(listenaddr, hookpath)

}

func initTemplate() {
	tmpllock.Lock()
	defer tmpllock.Unlock()

	tmpl = template.New("main")
	tmpl.Funcs(template.FuncMap{
		"truncate": func(s string, l int, endstring string) (ret string) {
			if len(s) > l {
				ret = s[0:l-len(endstring)] + endstring
			} else {
				ret = s
			}
			return
		},
		"trim":     strings.TrimSpace,
		"unescape": html.UnescapeString,
	})
}

// needs to be called with locks held!
func saveState() {
	buff := new(bytes.Buffer)
	enc := gob.NewEncoder(buff)
	err := enc.Encode(state)
	if err != nil {
		D("Error encoding state:", err)
	}
	err = ioutil.WriteFile(".state.dc", buff.Bytes(), 0600)
	if err != nil {
		D("Error with writing out state file:", err)
	}
}

func loadState() {
	statelock.Lock()
	defer statelock.Unlock()

	contents, err := ioutil.ReadFile(".state.dc")
	if err != nil {
		D("Error while reading from state file")
		initState()
		return
	}
	buff := bytes.NewBuffer(contents)
	dec := gob.NewDecoder(buff)
	err = dec.Decode(&state)

	if err != nil {
		D("Error decoding state, initializing", err)
	}
	initState()

	// migrate the Seenrss keys to their md5 hashed versions
	for oldkey, value := range state.Seenrss {
		if len(oldkey) > 24 { // if longer, it needs converting
			newkey := getHash(oldkey)
			state.Seenrss[newkey] = value
			delete(state.Seenrss, oldkey)
		}
	}
}

func initState() {

	if state.Factoids == nil {
		state.Factoids = make(map[string]string)
	}

	if state.Seenrss == nil {
		state.Seenrss = make(map[string]int64)
	}

	if state.Admins == nil || !state.Admins["melkor"] {
		state.Admins = map[string]bool{
			"melkor":                       true,
			"sztanpet.users.quakenet.org":  true,
			"R1CH.users.quakenet.org":      true,
			"Jim.users.quakenet.org":       true,
			"Warchamp7.users.quakenet.org": true,
			"hwd.users.quakenet.org":       true,
			"paibox.users.quakenet.org":    true,
			"ThoNohT.users.quakenet.org":   true,
			"dodgepong.users.quakenet.org": true,
			"Sapiens.users.quakenet.org":   true,
		}
	}
}

func getHash(data string) string {
	hash := md5.Sum([]byte(data))
	return base64.StdEncoding.EncodeToString(hash[:])
}
