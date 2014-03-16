package main

import (
	"bufio"
	"bytes"
	"fmt"
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
	isalpha     = regexp.MustCompile(`^[a-zA-Z0-9-.]+$`)
)

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
			srv.raw("JOIN #obs-dev")
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

		var isadmin bool
		if len(m.Host) > 0 {
			statelock.Lock()
			isadmin = state.Admins[m.Host]
			statelock.Unlock()
		}

		// handle administering the factoids
		if isadmin {
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
			statelock.Lock()
			defer statelock.Unlock()
			if factoidkey == "list" {
				factoidlist := make([]string, 0, len(state.Factoids))
				for k, _ := range state.Factoids {
					factoidlist = append(factoidlist, strings.ToLower(k))
				}
				sort.Strings(factoidlist)
				srv.raw("PRIVMSG ", target, " :", strings.Join(factoidlist, ", "))
			} else if factoid, ok := state.Factoids[factoidkey]; ok && isalpha.MatchString(factoidkey) {
				if pos != len(m.Message) { // there was a postfix
					rest := m.Message[pos+1:]      // skip the space
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
			tryHandleAnalyzer(m)
		}
	}
}

func (srv *IRC) handleAdminMessage(m *Message) {

	if m.Message[0:1] != "." {
		return
	}

	s := strings.SplitN(m.Message[1:], " ", 3)
	if len(s) < 2 || !isalpha.MatchString(s[1]) {
		return
	}

	statelock.Lock()
	defer statelock.Unlock()
	var factoidModified bool
	command := s[0]
	factoidkey := s[1]
	var factoid string
	if len(s) == 3 {
		factoid = s[2]
	}

	switch command {
	case "addadmin":
		// first argument is the host to match
		state.Admins[s[1]] = true
		srv.raw("NOTICE ", m.Nick, " :Added host successfully")
		factoidModified = true
	case "deladmin":
		delete(state.Admins, s[1])
		srv.raw("NOTICE ", m.Nick, " :Removed host successfully")
		factoidModified = true
	case "add":
		fallthrough
	case "mod":
		if len(s) != 3 {
			return
		}
		state.Factoids[factoidkey] = factoid
		factoidModified = true
		srv.raw("NOTICE ", m.Nick, " :Added/Modified successfully")
	case "del":
		if _, ok := state.Factoids[factoidkey]; ok {
			srv.raw("NOTICE ", m.Nick, " :Deleted successfully")
		}
		delete(state.Factoids, factoidkey)
		factoidModified = true
	case "raw":
		// execute anything received from the private message with the command raw
		srv.raw(factoidkey, " ", factoid)
	default:
		return
	}

	if factoidModified {
		saveState()
	}
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

	t := time.NewTicker(time.Second)
	for _, c := range lines {
		if todevchan {
			srv.raw("PRIVMSG #obs-dev :", c)
		} else {
			srv.raw("PRIVMSG #obsproject :", c)
		}
		<-t.C
	}
	t.Stop()
}

func initIRC(addr string) {
	srv.Addr = addr
	go srv.run()
}
