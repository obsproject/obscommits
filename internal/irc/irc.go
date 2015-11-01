/***
  This file is part of obscommits.

  Copyright (c) 2015 Peter Sztan <sztanpet@gmail.com>

  obscommits is free software; you can redistribute it and/or modify it
  under the terms of the GNU Lesser General Public License as published by
  the Free Software Foundation; either version 3 of the License, or
  (at your option) any later version.

  obscommits is distributed in the hope that it will be useful, but
  WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
  Lesser General Public License for more details.

  You should have received a copy of the GNU Lesser General Public License
  along with obscommits; If not, see <http://www.gnu.org/licenses/>.
***/

package irc

import (
	"fmt"
	"math"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/sorcix/irc"
	"github.com/sztanpet/obscommits/internal/config"
	"github.com/sztanpet/obscommits/internal/debug"
	"golang.org/x/net/context"
)

var contextKey *int

func init() {
	contextKey = new(int)
}

// copied from github.com/sorcix/irc
// to make the package usable standalone
type Prefix irc.Prefix
type Message irc.Message

// Callback is the way to handle events from the irc server.
// The callback should return true only if the library should skip the
// processing of the event (currently only nick-in-use is handled by the lib)
// It is not possible to cancel RPL_WELCOME or PING handling,
// the pings we send have the time formatted as time.RFC3339Nano sent as
// the parameter (so it's possible to measure the latency on PONG).
//
// If you want to send messages before being logged-in to the server, you have
// to use WriteImmediate, be aware that the method does not use ratelimiting.
type Callback func(*IConn, *Message) bool

// IConn represents the IRC connection, methods are safe to call concurrently
type IConn struct {
	cb Callback

	mu   sync.Mutex
	conn net.Conn
	quit chan struct{}
	w    chan *irc.Message

	wg sync.WaitGroup
	*irc.Decoder
	*irc.Encoder
	cfg config.IRC

	// exponentially increase the time we sleep based on the number of tries
	// only resets when successfully connected to the server
	tries float64
	// the number of pings that were sent but not yet answered, should never go
	// beyond 2
	pendingPings int

	// for ratelimiting purposes
	badness  time.Duration
	lastsent time.Time
}

func Init(ctx context.Context, cb Callback) context.Context {
	cfg := config.FromContext(ctx).IRC
	c := &IConn{
		cb:   cb,
		cfg:  cfg,
		w:    make(chan *irc.Message),
		quit: make(chan struct{}),
	}
	ctx = context.WithValue(ctx, contextKey, c)
	c.Reconnect("init")

	return ctx
}

func FromContext(ctx context.Context) *IConn {
	c, _ := ctx.Value(contextKey).(*IConn)
	return c
}

func ParseMessage(raw string) (m *Message) {
	m = (*Message)(irc.ParseMessage(raw))
	return
}

// Reconnect reconnects to the server
func (c *IConn) Reconnect(format string, args ...interface{}) {
	c.mu.Lock()

	close(c.quit)
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.wg.Wait()

	// incremental backoff with a maximum backoff of ~5 minutes
	if c.tries > 0 {
		d := time.Duration(math.Pow(2.0, c.tries)*300) * time.Millisecond
		c.logWithDuration(format, d, args...)
		time.Sleep(d)
	}

	c.quit = make(chan struct{})
	conn, err := net.DialTimeout("tcp", c.cfg.Addr, 5*time.Second)
	if err != nil {
		c.mu.Unlock()
		c.addTries()
		c.Reconnect("conn error: %+v", err)
		return
	}

	c.pendingPings = 0
	c.badness = 0
	c.conn = conn
	c.Decoder = irc.NewDecoder(conn)
	c.Encoder = irc.NewEncoder(conn)

	c.wg.Add(1)
	go c.readLoop()

	defer (func() {
		c.mu.Unlock()
		if err != nil {
			c.addTries()
			c.Reconnect("write error: %+v", err)
		}
	})()

	err = c.WriteImmediate(&Message{
		Command: irc.USER,
		Params: []string{
			c.cfg.Ident,
			"0",
			"*",
		},
		Trailing: c.cfg.UserName,
	})
	if err != nil {
		return
	}
	err = c.WriteImmediate(&Message{Command: irc.NICK, Params: []string{c.cfg.Nick}})
	if err != nil {
		return
	}
	if len(c.cfg.Password) > 0 {
		err = c.WriteImmediate(&Message{Command: irc.PASS, Params: []string{c.cfg.Password}})
		if err != nil {
			return
		}
	}
}

// PrivMsg sends a PRIVMSG to the designated target with the following trailing
// arguments
func (c *IConn) PrivMsg(im *Message, args ...string) {
	target := c.Target(im)
	m := &Message{
		Command:  irc.PRIVMSG,
		Params:   []string{target},
		Trailing: strings.Join(args, ""),
	}
	c.Write(m)
}

// Notice sends a NOTICE to the designated target with the following trailing
// arguments
func (c *IConn) Notice(im *Message, args ...string) {
	m := &Message{
		Command:  irc.NOTICE,
		Params:   []string{im.Prefix.Name},
		Trailing: strings.Join(args, ""),
	}
	c.Write(m)
}

// Write handles sending messages, it reconnects if there are problems
// It applies default ratelimiting to any message, and will not send messages
// before being logged-in to the server
func (c *IConn) Write(m *Message) {
	im := (*irc.Message)(m)
	if t := c.rateLimit(im.Len()); t != 0 {
		<-time.After(t)
	}

	c.w <- im
}

func (c *IConn) WriteLines(target string, lines []string, showlast bool) {
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

	for _, l := range lines {
		c.Write(&Message{
			Command:  irc.PRIVMSG,
			Params:   []string{target},
			Trailing: l,
		})
	}
}

// WriteImmediate is a way to write to the server before being logged-in,
// it does not use ratelimiting and does nothing extra
func (c *IConn) WriteImmediate(m *Message) error {
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	msg := (*irc.Message)(m)
	if err := c.Encode(msg); err != nil {
		return err
	}

	return nil
}

func (c *IConn) addTries() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// clamp tries, so that the maximum amount of time we wait is ~5 minutes
	if c.tries > 10.0 {
		c.tries = 10.0
	}

	c.tries++
}

func (c *IConn) logWithDuration(format string, dur time.Duration, args ...interface{}) {
	newargs := make([]interface{}, 0, len(args)+1)
	newargs = append(newargs, args...)
	newargs = append(newargs, dur)
	d.PF(2, format+", reconnecting in %s", newargs...)
}

func (c *IConn) writeLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.quit:
			return
		case m := <-c.w:
			d.DF(1, "\t> %v", m.String())
			if err := c.WriteImmediate((*Message)(m)); err != nil {
				c.addTries()
				go c.Reconnect("write error: %+v", err)
				return
			}
		}
	}
}

// read handles parsing messages from IRC and reconnects if there are problems
// returns nil on error
func (c *IConn) readLoop() {
	defer c.wg.Done()

	for {
		// if there are pending pings, lower the timeout duration to speed up
		// the disconnection
		if c.pendingPings > 0 {
			_ = c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		} else {
			_ = c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		}

		m, err := c.Decode()

		select {
		case <-c.quit:
			return
		default:
		}

		if err == nil {
			// we do not actually care about the type of the message the server
			// sends us, as long as it sends something it signals that its alive
			if c.pendingPings > 0 {
				c.pendingPings--
			}

			switch m.Command {
			case irc.PING:
				c.WriteImmediate(&Message{Command: irc.PONG, Params: m.Params, Trailing: m.Trailing})
			case irc.RPL_WELCOME: // successfully connected
				d.PF(1, "Successfully connected to IRC")
				c.mu.Lock()
				c.tries = 0

				// only start processing write requests after we are logged in
				c.wg.Add(1)
				go c.writeLoop()

				c.mu.Unlock()

				if len(c.cfg.Channels) > 0 {
					for _, ch := range c.cfg.Channels {
						c.Write(&Message{Command: irc.JOIN, Params: []string{ch}})
					}
				}
			}

			// if callback returned true -> skip our own processing
			if c.cb != nil && c.cb(c, (*Message)(m)) {
				d.DF(1, "\t< %v", m.String())
				continue
			}

			switch m.Command {
			case irc.ERR_NICKNAMEINUSE:
				c.w <- &irc.Message{
					Command: irc.NICK,
					Params: []string{
						fmt.Sprintf(c.cfg.Nick+"%d", rand.Intn(10)),
					},
				}
			default:
				d.DF(1, "\t< %v", m.String())
			}

			continue
		}

		// if we hit the timeout and there are no outstanding pings, send one
		if e, ok := err.(net.Error); ok && e.Timeout() && c.pendingPings < 1 {
			c.pendingPings++
			c.w <- &irc.Message{
				Command: "PING",
				Params:  []string{time.Now().Format(time.RFC3339Nano)},
			}

			continue
		}

		// otherwise there either was an error or we did not get a reply for our ping
		c.addTries()
		go c.Reconnect("read error: %+v", err)
		return
	}
}

// rateLimit implements Hybrid's flood control algorithm for outgoing lines.
// Copyright (c) 2009+ Alex Bramley, github.com/fluffle/goirc
func (c *IConn) rateLimit(chars int) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Hybrid's algorithm allows for 2 seconds per line and an additional
	// 1/120 of a second per character on that line.
	linetime := 2*time.Second + time.Duration(chars)*time.Second/120
	elapsed := time.Now().Sub(c.lastsent)
	if c.badness += linetime - elapsed; c.badness < 0 {
		// negative badness times are badness...
		c.badness = 0
	}
	c.lastsent = time.Now()
	// If we've sent more than 10 second's worth of lines according to the
	// calculation above, then we're at risk of "Excess Flood".
	if c.badness > 10*time.Second {
		return linetime
	}
	return 0
}

func (c *IConn) Target(m *Message) string {
	if len(m.Params) == 0 || len(m.Params[0]) == 0 {
		return ""
	}

	target := m.Params[0]
	// FIXME way too naive of a way to see if we got a private message or not
	if target[0] == '#' {
		return target
	}

	// not a channel message, so its a private message, so return the sender
	return m.Prefix.Name
}
