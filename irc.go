package main

import (
	"bytes"
	"fmt"
	irc "github.com/fluffle/goirc/client"
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type IRC struct {
	Addr string
	Conn *irc.Conn
	buff bytes.Buffer
	sync.RWMutex
}

var srv = IRC{}
var usedfactoids = map[string]time.Time{}
var (
	isalpha = regexp.MustCompile(`^[a-zA-Z0-9-.]+$`)
)

func initIRC(addr string) {
	srv.Init(addr)
}

func (srv *IRC) raw(s ...string) {
	srv.Lock()
	defer srv.Unlock()

	srv.buff.Reset()
	for _, v := range s {
		srv.buff.Write([]byte(v))
	}
	srv.buff.Write([]byte("\r\n"))
	srv.Conn.Raw(srv.buff.String())

}

func (srv *IRC) handleLines(lines []string, showlast bool, todevchan bool) {
	l := len(lines)

	if l == 0 {
		return
	}

	if l > 5 {
		if showlast {
			lines = lines[l-5:]
		} else {
			lines = lines[:5]
		}
	}

	// flood control is handled by the goirc lib
	for _, c := range lines {
		if todevchan {
			srv.raw("PRIVMSG #obs-dev :", c)
		} else {
			srv.raw("PRIVMSG #obsproject :", c)
		}
	}
}

func (srv *IRC) Init(addr string) {

	cfg := irc.NewConfig("OBScommits")
	cfg.Me.Ident = "obscommits"
	cfg.Me.Name = "github.com/sztanpet/obscommits"
	cfg.Server = addr
	cfg.NewNick = srv.NewNick
	srv.Addr = addr
	c := irc.Client(cfg)
	c.EnableStateTracking()
	c.HandleFunc(irc.DISCONNECTED, srv.onDisconnect)
	c.HandleFunc(irc.CONNECTED, srv.onConnect)
	c.HandleFunc(irc.PRIVMSG, srv.onMessage)
	srv.Conn = c
	srv.Connect()
}

func (srv *IRC) NewNick(nick string) string {
	return fmt.Sprintf("OBScommits%d", rand.Intn(10))
}

func (srv *IRC) onDisconnect(c *irc.Conn, line *irc.Line) {
	srv.Init(srv.Addr)
}

func (srv *IRC) Connect() {
	err := srv.Conn.Connect()
	if err != nil {
		D("Connection error:", err, "reconnecting in 30 seconds")
		<-time.After(30 * time.Second)
		srv.Connect()
	}
}

func (srv *IRC) onConnect(c *irc.Conn, line *irc.Line) {
	c.Join("#obsproject")
	c.Join("#obs-dev")
}

func (srv *IRC) onMessage(c *irc.Conn, line *irc.Line) {

	var isadmin bool
	if len(line.Host) > 0 {
		statelock.Lock()
		isadmin = state.Admins[line.Host]
		statelock.Unlock()
	}

	// handle administering the factoids
	if isadmin && srv.onAdminMessage(line) {
		return
	}

	// handle displaying of factoids
	message := line.Text()
	if len(message) > 0 && message[0:1] == "!" && len(line.Args) > 0 {
		target := line.Args[0]
		if target == c.Me().Nick { // if we are the recipients, its a private message
			target = line.Nick // so send it back privately too
		}
		pos := strings.Index(message, " ")
		if pos < 0 {
			pos = len(message)
		}
		factoidkey := message[1:pos]
		statelock.Lock()
		defer statelock.Unlock()
		if factoidkey == "list" {
			if factoidUsedRecently(factoidkey) {
				return
			}
			factoidlist := make([]string, 0, len(state.Factoids))
			for k, _ := range state.Factoids {
				factoidlist = append(factoidlist, strings.ToLower(k))
			}
			sort.Strings(factoidlist)
			srv.raw("PRIVMSG ", target, " :", strings.Join(factoidlist, ", "))
		} else if factoid, ok := state.Factoids[factoidkey]; ok && isalpha.MatchString(factoidkey) {
			if factoidUsedRecently(factoidkey) {
				return
			}
			if pos != len(message) { // there was a postfix
				rest := message[pos+1:]        // skip the space
				pos = strings.Index(rest, " ") // and search for the next space
				if pos > 0 {                   // and only print the first thing delimeted by a space
					rest = rest[0:pos]
				}
				srv.raw("PRIVMSG ", target, " :", rest, ": ", factoid)
			} else { // otherwise just print the factoid
				srv.raw("PRIVMSG ", target, " :", factoid)
			}
		}
	} else {
		tryHandleAnalyzer(line.Nick, message)
	}
}

func (srv *IRC) onAdminMessage(line *irc.Line) bool {

	message := line.Text()
	if len(message) > 0 && message[0:1] != "." {
		return false
	}

	s := strings.SplitN(message[1:], " ", 4)
	if len(s) < 2 || !isalpha.MatchString(s[1]) {
		return false
	}

	statelock.Lock()
	defer statelock.Unlock()
	var factoidModified bool
	command := s[0]
	factoidkey := s[1]
	var factoidkey2 strings
	var factoid string
	if len(s) >= 3 {
		factoidkey2 = s[2]
		factoid = s[2]
	}

	if len(s) == 4 {
		factoid = s[2] + " " + s[3]
	}

	switch command {
	case "addadmin":
		// first argument is the host to match
		state.Admins[s[1]] = true
		srv.raw("NOTICE ", line.Nick, " :Added host successfully")
		factoidModified = true
	case "deladmin":
		delete(state.Admins, s[1])
		srv.raw("NOTICE ", line.Nick, " :Removed host successfully")
		factoidModified = true
	case "add":
		fallthrough
	case "mod":
		if len(s) != 3 {
			return true
		}
		state.Factoids[factoidkey] = factoid
		factoidModified = true
		srv.raw("NOTICE ", line.Nick, " :Added/Modified successfully")
	case "del":
		if _, ok := state.Factoids[factoidkey]; ok {
			srv.raw("NOTICE ", line.Nick, " :Deleted successfully")
		}
		delete(state.Factoids, factoidkey)
		factoidModified = true
	case "rename":
		if _, ok := state.Factoids[factoidkey2]; ok {
			srv.raw("NOTICE ", line.Nick, " :Renaming would overwrite, please delete first")
			return true
		}
		if _, ok := state.Factoids[factoidkey]; ok {
			state.Factoids[factoidkey2] = state.Factoids[factoidkey]
			delete(state.Factoids, factoidkey)
			factoidModified = true
			srv.raw("NOTICE ", line.Nick, " :Renamed successfully")
		} else {
			srv.raw("NOTICE ", line.Nick, " :Not present")
		}
	case "raw":
		// execute anything received from the private message with the command raw
		srv.raw(factoidkey, " ", factoid)
	default:
		return false
	}

	if factoidModified {
		saveState()
	}
	return true
}

func factoidUsedRecently(factoidkey string) bool {
	if lastused, ok := usedfactoids[factoidkey]; ok && time.Since(lastused) < 30*time.Second {
		D("Not handling factoid:", factoidkey, ", because it was used too recently!")
		usedfactoids[factoidkey] = time.Now()
		return true
	}
	usedfactoids[factoidkey] = time.Now()
	return false
}
