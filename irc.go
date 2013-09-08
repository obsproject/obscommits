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
	"runtime"
	"sort"
	"strings"
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
	ping      chan bool
	write     chan string
	stop      chan bool
	restart   chan bool
	connected bool
}

var srv = IRC{
	nick:    "OBScommits",
	stop:    make(chan bool),
	restart: make(chan bool),
}

var (
	// 1 prefix
	// 2 command
	// 3 arguments
	ircre       = regexp.MustCompile(`^(?:[:@]([^ ]+) +)?(?:([^\r\n ]+) *)([^\r\n]*)[\r\n]{1,2}$`)
	ircargre    = regexp.MustCompile(`(.*?) :(.*)$`)
	ircprefixre = regexp.MustCompile(`^([^!]*)(?:!([^@]+)@(.*))?$`)
)

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
	} else if matches[3] != "" && matches[3][0:1] == ":" {
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

	if srv.write == nil {
		D("Tried writing when the write channel was closed")
		return
	}

	var b bytes.Buffer
	for _, v := range s {
		b.Write([]byte(v))
	}
	b.Write([]byte("\r\n"))
	srv.write <- b.String()

}

const (
	TIMEOUT           = 30 * time.Second
	RECONNECTDURATION = 30 * time.Second
)

func (srv *IRC) writer(conn net.Conn) {
	var okayuntil time.Time
	ping := time.NewTimer(TIMEOUT)
	defer (func() {
		D("Deferred writer closing, closing connection")
		ping.Stop()
		conn.Close()
	})()

	for {
		select {
		case <-srv.ping:
			okayuntil = time.Now().Add(TIMEOUT + 5*time.Second)
			ping.Reset(TIMEOUT)
		case now := <-ping.C:
			srv.raw(fmt.Sprintf("PING %d", now.UnixNano()))
			ping.Reset(TIMEOUT)
			if now.After(okayuntil) {
				P("Timed out, reconnecting in 30sec")
				return
			}
			runtime.GC()
		case b, ok := <-srv.write:
			if !ok {
				srv.write = nil
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

reconnect:
	srv.ping = make(chan bool, 4)
	srv.write = make(chan string, 1)
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

	go srv.writer(conn)
	srv.raw("NICK ", srv.nick)
	srv.raw("USER ", srv.nick, " x x :github.com/sztanpet/obscommits")

	buff := bufio.NewReader(conn)
	for {
		line, err := buff.ReadString('\n')
		if err != nil {
			P("Read error: ", err)
			if srv.write != nil {
				D("Closing write channel")
				close(srv.write)
			}
			P("Reconnecting in 30sec")
			time.Sleep(RECONNECTDURATION)
			goto reconnect
		}
		DP("< ", line)
		srv.ping <- true
		m := srv.parse(line)
		srv.handleMessage(m)
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
	case "NICK":
		if m.Nick == srv.nick {
			srv.nick = m.Message
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
			target := m.Parameters[0]
			if target == srv.nick { // if we are the recipients, its a private message
				target = m.Nick // so send it back privately too
			}
			pos := strings.Index(m.Message, " ")
			if pos < 0 {
				pos = len(m.Message)
			}
			factoidkey := m.Message[1:pos]
			if factoidkey == "list" {
				factoidlist := make([]string, 0, len(factoids))
				for k, _ := range factoids {
					factoidlist = append(factoidlist, strings.ToLower(k))
				}
				sort.Strings(factoidlist)
				srv.raw("PRIVMSG ", target, " :", strings.Join(factoidlist, ", "))
			} else if factoid, ok := factoids[factoidkey]; ok {
				srv.raw("PRIVMSG ", target, " :", factoid)
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
	case "raw":
		// execute anything received from the private message with the command raw
		srv.raw(factoidkey, " ", factoid)
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

func (srv *IRC) handleLines(lines []string, showlast bool) {

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

	t := time.NewTicker(time.Second)
	for _, c := range lines {
		srv.raw("PRIVMSG #obsproject :", c)
		<-t.C
	}
	t.Stop()
}

func initIRC(addr string) {
	srv.Addr = addr
	srv.loadFactoids()
	go srv.run()
}
