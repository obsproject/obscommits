package main

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"regexp"
	"strings"
	"text/template"
	"time"
)

type Message struct {
	Prefix     string
	Nick       string
	Ident      string
	Host       string
	Command    string
	Parameters []string
	Message    string
}

type IRC struct {
	Addr      string
	nick      string
	read      chan string
	write     chan string
	stop      chan bool
	restart   chan bool
	connected bool
}

var srv IRC
var (
	// 1 prefix
	// 2 command
	// 3 arguments
	ircre       = regexp.MustCompile(`^(?:[:@]([^ ]+)[ ]+)?(?:([^ ]+)[ ]+)([^\r\n]*)[\r\n]{1,2}$`)
	ircargre    = regexp.MustCompile(`(.*) :(.*)$`)
	ircprefixre = regexp.MustCompile(`^([^!]*)(?:!([^@]+)@(.*))?$`)
)

var committpl *template.Template
var admins = map[string]bool{
	"melkor":                       true,
	"sztanpet.users.quakenet.org":  true,
	"R1CH.users.quakenet.org":      true,
	"Jim.users.quakenet.org":       true,
	"Warchamp7.users.quakenet.org": true,
	"hwd.users.quakenet.org":       true,
	"paibox.users.quakenet.org":    true,
}
var factoids = make(map[string]string)

func (srv *IRC) parse(b string) *Message {
	matches := ircre.FindStringSubmatch(b)
	if matches == nil {
		return nil
	}

	m := &Message{
		Prefix:  matches[1],
		Command: matches[2],
	}

	args := ircargre.FindStringSubmatch(matches[3])
	if args != nil {
		m.Parameters = strings.SplitN(args[1], " ", 15)
		m.Message = args[2]
	} else if matches[3][0:1] == ":" {
		m.Message = matches[3][1:]
	} else {
		m.Parameters = strings.SplitN(matches[3], " ", 15)
	}

	usermatches := ircprefixre.FindStringSubmatch(matches[1])
	if usermatches != nil {
		m.Nick = usermatches[1]
		m.Ident = usermatches[2]
		m.Host = usermatches[3]
	}

	return m
}

func (srv *IRC) raw(s ...string) {
	var b bytes.Buffer
	for _, v := range s {
		b.Write([]byte(v))
	}
	b.Write([]byte("\r\n"))
	srv.write <- b.String()
}

func (srv *IRC) reader(conn net.Conn) {
	defer (func() {
		srv.restart <- true
	})()
	buff := bufio.NewReader(conn)
	for {
		line, err := buff.ReadString('\n')
		if err != nil {
			P("Read error: ", err)
			if srv.write != nil {
				close(srv.write)
			}
			return
		}
		DP("< ", line)
		srv.read <- line
	}
}

func (srv *IRC) writer(conn net.Conn) {
	defer conn.Close() // so that the reading side gets unblocked
	for {
		select {
		case <-srv.stop:
			return
		case b := <-srv.write:
			if len(b) == 0 || srv.write == nil {
				return
			}
			DP("> ", b)
			written := 0
		write:
			conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
			n, err := conn.Write([]byte(b[written:]))
			if err != nil {
				P("Write error: ", err)
				return
			}
			written += n
			if written < len(b) {
				goto write
			}
		}
	}
}

func (srv *IRC) run() {
	srv.read = make(chan string, 256)
	var okayuntil time.Time
reconnect:
	srv.write = make(chan string)
	srv.connected = false

	addr, err := net.ResolveTCPAddr("tcp", srv.Addr)
	if err != nil {
		P("Failed to resolve irc addr, trying again in 5sec: ", err)
		time.Sleep(5 * time.Second)
		goto reconnect
	}

	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		P("Connection error, trying again in 5sec: ", err)
		time.Sleep(5 * time.Second)
		goto reconnect
	}

	conn.SetNoDelay(false)
	conn.SetKeepAlive(true)

	go srv.reader(conn)
	go srv.writer(conn)

	srv.raw("NICK ", srv.nick)
	srv.raw("USER ", srv.nick, " a a :", srv.nick)
	ping := time.NewTicker(30 * time.Second)
	for {
		select {
		case now := <-ping.C:
			srv.raw(fmt.Sprintf("PING %d", now.UnixNano()))
			if okayuntil.Before(now) {
				P("Timed out, reconnecting in 5sec")
				srv.stop <- true
				time.Sleep(5 * time.Second)
				goto reconnect
			}
		case <-srv.restart:
			P("Reconnecting in 5 seconds")
			time.Sleep(5 * time.Second)
			goto reconnect
		case b := <-srv.read:
			okayuntil = time.Now().Add(61 * time.Second)
			m := srv.parse(b)
			srv.handleMessage(m)
		}
	}
}

func (srv *IRC) handleMessage(m *Message) {
	switch m.Command {
	case "005":
		if !srv.connected {
			srv.connected = true
			// if we got 005, consider the server successfully connected, so join the channel
			srv.raw("JOIN #obsproject")
		}
	case "433":
		// nick in use
		srv.nick = fmt.Sprintf("OBScommits%d", rand.Intn(10))
		srv.raw("NICK ", srv.nick)
	case "PING":
		srv.raw("PONG :", m.Message)
	case "PRIVMSG":
		// handle administering the factoids
		if len(m.Host) > 0 && admins[m.Host] {
			srv.handleAdminMessage(m)
		}
		// handle displaying of factoids
		if m.Message[0:1] == "!" {
			pos := strings.Index(m.Message, " ")
			if pos < 0 {
				pos = len(m.Message)
			}
			factoidkey := m.Message[1:pos]
			if factoid, ok := factoids[factoidkey]; ok {
				srv.raw("PRIVMSG ", m.Parameters[0], " :", factoid)
			}
		}
	}
}

func (srv *IRC) handleAdminMessage(m *Message) {
	s := strings.SplitN(m.Message, " ", 3)
	if len(s) < 2 {
		return
	}

	command := s[0]
	factoidkey := s[1]
	var factoid string
	if len(s) == 3 {
		factoid = s[2]
	}

	switch command {
	case "add":
		fallthrough
	case "mod":
		if len(s) != 3 {
			return
		}
		factoids[factoidkey] = factoid
		srv.raw("NOTICE ", m.Nick, " :Added/Modified successfully")
	case "del":
		if _, ok := factoids[factoidkey]; ok {
			srv.raw("NOTICE ", m.Nick, " :Deleted successfully")
		}
		delete(factoids, factoidkey)
	default:
		return
	}

	srv.saveFactoids()
}

func (srv *IRC) loadFactoids() {
	n, err := ioutil.ReadFile("factoids.dc")
	if err != nil {
		D("Error while reading from factoids file")
		return
	}
	mb := bytes.NewBuffer(n)
	dec := gob.NewDecoder(mb)
	err = dec.Decode(&factoids)
	if err != nil {
		D("Error decoding factoids file")
	}
}

func (srv *IRC) saveFactoids() {
	mb := new(bytes.Buffer)
	enc := gob.NewEncoder(mb)
	enc.Encode(&factoids)
	err := ioutil.WriteFile("factoids.dc", mb.Bytes(), 0600)
	if err != nil {
		D("Error with writing out factoids file:", err)
	}
}

func (srv *IRC) handleCommits(commits []*Commit) {
	t := time.NewTicker(time.Second)
	for _, c := range commits {
		b := bytes.NewBufferString("")
		committpl.Execute(b, c)
		srv.raw("PRIVMSG #obsproject :", b.String())
		<-t.C
	}
}

func initIRC(addr string) {
	committpl = template.New("test")
	committpl.Funcs(template.FuncMap{
		"truncate": func(s string, l int, endstring string) (ret string) {
			if len(s) > l {
				ret = s[0:l-len(endstring)] + endstring
			} else {
				ret = s
			}
			return
		},
	})
	committpl = template.Must(committpl.Parse(`[{{.Branch}}|{{.Author}}] {{truncate .Message 200 "..."}} https://github.com/jp9000/OBS/commit/{{truncate .ID 7 ""}}`))

	srv = IRC{
		Addr:    addr,
		nick:    "OBScommits",
		stop:    make(chan bool),
		restart: make(chan bool),
	}

	srv.loadFactoids()
	go srv.run()
}
