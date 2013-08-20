package main

import (
	conf "github.com/msbranco/goconfig"
)

var debuggingenabled = true

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

	initIRC(ircaddr)
	initGithub(listenaddr, hookpath)

}
