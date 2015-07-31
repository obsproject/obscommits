package main

import (
	"net/http"

	irclogger "github.com/fluffle/goirc/logging"
	conf "github.com/msbranco/goconfig"
)

var debuggingenabled = true
var state State
var tmpl Template

func main() {
	irclogger.SetLogger(debugLogger{})
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
		nc.AddOption("rss", "githubnewsurl", "dunce://need.to.set.it")
		nc.AddSection("factoids")
		nc.AddOption("factoids", "hookpath", "/factoids")

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
	githookpath, _ := c.GetString("git", "hookpath")
	factoidhookpath, _ := c.GetString("factoids", "hookpath")
	rssurl, _ = c.GetString("rss", "url")

	state.init()
	tmpl.init()
	initIRC(ircaddr)

	initRSS()
	initFactoids(factoidhookpath)
	initGithub(githookpath)

	if err := http.ListenAndServe(listenaddr, nil); err != nil {
		F("ListenAndServe:", err)
	}
}
