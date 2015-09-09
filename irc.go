package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"time"

	irc "github.com/fluffle/goirc/client"
	"github.com/sztanpet/obscommits/internal/debug"
	"github.com/sztanpet/obscommits/internal/persist"
)

type IRC struct {
	Addr string
	Conn *irc.Conn
	buff bytes.Buffer
	sync.RWMutex
}

var srv = IRC{}
var (
	adminState *persist.State
	admins     map[string]struct{}
)
var (
	isalpha = regexp.MustCompile(`^[a-zA-Z0-9-.]+$`)
)

func initIRC(addr string) {
	var err error
	adminState, err = persist.New("admins.state", &map[string]struct{}{
		"melkor":                       struct{}{},
		"sztanpet.users.quakenet.org":  struct{}{},
		"R1CH.users.quakenet.org":      struct{}{},
		"Jim.users.quakenet.org":       struct{}{},
		"Warchamp7.users.quakenet.org": struct{}{},
		"hwd.users.quakenet.org":       struct{}{},
		"paibox.users.quakenet.org":    struct{}{},
		"ThoNohT.users.quakenet.org":   struct{}{},
		"dodgepong.users.quakenet.org": struct{}{},
		"Sapiens.users.quakenet.org":   struct{}{},
	})
	if err != nil {
		d.F(err.Error())
	}

	admins = adminState.Get().(map[string]struct{})
	srv.Init(addr)
}

func (srv *IRC) raw(s ...string) {
	srv.Lock()
	defer srv.Unlock()

	srv.buff.Reset()
	for _, v := range s {
		srv.buff.WriteString(v)
	}
	srv.buff.WriteString("\r\n")

	srv.Conn.Raw(srv.buff.String())
}

func (srv *IRC) privmsg(target string, s ...string) {
	srv.Lock()
	defer srv.Unlock()

	srv.buff.Reset()
	for _, v := range s {
		srv.buff.Write([]byte(v))
	}

	srv.Conn.Privmsg(target, srv.buff.String())
}

func (srv *IRC) notice(target string, s ...string) {
	srv.Lock()
	defer srv.Unlock()

	srv.buff.Reset()
	for _, v := range s {
		srv.buff.Write([]byte(v))
	}

	srv.Conn.Notice(target, srv.buff.String())
}

func (srv *IRC) handleLines(target string, lines []string, showlast bool) {
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
		srv.privmsg(target, c)
	}
}

func (srv *IRC) Init(addr string) {

	cfg := irc.NewConfig("OBScommits")
	cfg.Me.Ident = "obscommits"
	cfg.Me.Name = "http://obscommits.sztanpet.net/factoids"
	cfg.Server = addr
	cfg.NewNick = srv.NewNick
	cfg.SplitLen = 430
	srv.Addr = addr
	c := irc.Client(cfg)
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
		d.D("Connection error:", err, "reconnecting in 30 seconds")
		<-time.After(30 * time.Second)
		srv.Connect()
	}
}

func (srv *IRC) onConnect(c *irc.Conn, line *irc.Line) {
	c.Join("#obsproject")
	c.Join("#obs-dev")
}

func (srv *IRC) onMessage(c *irc.Conn, line *irc.Line) {

	if len(line.Args) == 0 {
		return
	}

	target := line.Args[0]
	if target == c.Me().Nick { // if we are the recipients, its a private message
		target = line.Nick // so send it back privately too
	}

	message := line.Text()
	var isadmin bool
	if len(line.Host) > 0 {
		adminState.Lock()
		_, isadmin = admins[line.Host]
		adminState.Unlock()
	}

	// handle administering the factoids
	if isadmin && srv.onAdminMessage(target, line.Nick, message) {
		return
	}

	if tryHandleFactoid(target, message) == true {
		return
	}
	if tryHandleAnalyzer(target, line.Nick, message) == true {
		return
	}
}

func (srv *IRC) onAdminMessage(target, nick, message string) (abort bool) {

	if len(message) == 0 && message[0:1] != "." {
		return
	}

	s := strings.SplitN(message[1:], " ", 4)
	if len(s) < 2 || !isalpha.MatchString(s[1]) {
		return false
	}

	abort = tryHandleAdminFactoid(target, nick, s)
	if abort {
		return
	}

	switch s[0] {
	case "addadmin":
		// first argument is the host to match
		adminState.Lock()
		admins[s[1]] = struct{}{}
		adminState.Unlock()
		adminState.Save()
		srv.notice(nick, "Added host successfully")
	case "deladmin":
		adminState.Lock()
		delete(admins, s[1])
		adminState.Unlock()
		adminState.Save()
		srv.notice(nick, "Removed host successfully")
	case "raw":
		// execute anything received from the private message with the command raw
		msg := s[2]
		if len(s) == 4 {
			msg += " " + s[3]
		}
		srv.raw(s[1], " ", msg)
	}

	return
}
