package main

import (
	"bytes"
	"encoding/gob"
	conf "github.com/msbranco/goconfig"
	"io/ioutil"
	"strings"
	"sync"
	"text/template"
)

var debuggingenabled = true
var (
	state     = make(map[string]string)
	statelock = sync.RWMutex{}
)
var (
	tmpl     *template.Template
	tmpllock = sync.RWMutex{}
)

func main() {
	c, err := conf.ReadConfigFile("settings.cfg")
	if err != nil {
		nc := conf.NewConfigFile()
		nc.AddOption("default", "debug", "false")
		nc.AddOption("default", "ircserver", "irc.quakenet.org:6667")
		nc.AddOption("default", "listenaddress", ":9998")
		nc.AddOption("default", "githubhookpath", "/whatever")

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
	hookpath, _ := c.GetString("default", "githubhookpath")

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
		"trim": strings.TrimSpace,
	})
}

// need to be called with locks held!
func saveState() {
	mb := new(bytes.Buffer)
	enc := gob.NewEncoder(mb)
	enc.Encode(&state)
	err := ioutil.WriteFile(".state.dc", mb.Bytes(), 0600)
	if err != nil {
		D("Error with writing out state file:", err)
	}
}

func loadState() {
	statelock.Lock()
	defer statelock.Unlock()

	n, err := ioutil.ReadFile(".state.dc")
	if err != nil {
		D("Error while reading from state file")
		return
	}
	mb := bytes.NewBuffer(n)
	dec := gob.NewDecoder(mb)
	err = dec.Decode(&state)
	if err != nil {
		D("Error decoding state file", err)
	}
}
