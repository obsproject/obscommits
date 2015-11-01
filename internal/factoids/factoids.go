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

package factoids

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/sztanpet/obscommits/internal/config"
	"github.com/sztanpet/obscommits/internal/debug"
	"github.com/sztanpet/obscommits/internal/irc"
	"github.com/sztanpet/obscommits/internal/persist"
	"golang.org/x/net/context"
)

type factoid struct {
	Name    string
	Text    string
	Aliases []string
}

type factoidSlice []factoid

func (f factoidSlice) Len() int           { return len(f) }
func (f factoidSlice) Less(i, j int) bool { return f[i].Name < f[j].Name }
func (f factoidSlice) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }

var (
	alphaRE  = regexp.MustCompile(`^[a-zA-Z0-9-.]+$`)
	handleRE = regexp.MustCompile(`^!([a-zA-Z0-9-.]+)(?:\s+(\S+)\s*)?$`)
	adminRE  = regexp.MustCompile(`^\.(add|mod|del|rename|addalias|modalias|delalias)(?:\s+([a-zA-Z0-9-.]+)\s*)(?:(\S+))?(?:(.+))?$`)
	factoids = map[string]string{}
	aliases  = map[string]string{}
	used     = map[string]time.Time{}
	state    *persist.State
)

func Init(ctx context.Context) context.Context {
	handleRE.Longest()
	adminRE.Longest()

	type s struct {
		Factoids map[string]string
		Aliases  map[string]string
		Used     map[string]time.Time
	}

	var err error
	state, err = persist.New("factoids.state", &s{
		Factoids: factoids,
		Aliases:  aliases,
		Used:     used,
	})
	if err != nil {
		d.F(err.Error())
	}

	st := state.Get().(*s)
	factoids = st.Factoids
	aliases = st.Aliases
	used = st.Used

	tpl.init()
	path := config.FromContext(ctx).Factoids.HookPath
	http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		tpl.render()
		tpl.execute(w)
	})

	return ctx
}

func factoidUsedRecently(factoidkey string) (ret bool) {
	if lastused, ok := used[factoidkey]; ok && time.Since(lastused) < 30*time.Second {
		ret = true
	}
	used[factoidkey] = time.Now()
	return
}

// checks if there is a factoid, if there isnt tries to look if its an alias
// and then recurses with the found factoid
// the state lock needs to be held by the caller
func getfactoidByKey(factoidkey string) (factoid, key string, ok bool) {
	key = factoidkey
restart:
	if factoid, ok = factoids[key]; ok {
		return
	}
	key, ok = aliases[key]
	if ok {
		goto restart
	}

	return
}

func Handle(c *irc.IConn, m *irc.Message) (abort bool) {
	matches := handleRE.FindStringSubmatch(m.Trailing)
	if len(matches) == 0 {
		return
	}

	factoidkey := strings.ToLower(matches[1])

	state.Lock()
	defer state.Unlock()
	if factoid, factoidkey, ok := getfactoidByKey(factoidkey); ok {
		abort = true
		if factoidUsedRecently(factoidkey) {
			return
		}
		if len(matches[2]) > 0 { // someone is being sent a factoid
			c.PrivMsg(m, matches[2], ": ", factoid)
		} else { // otherwise just print the factoid
			c.PrivMsg(m, factoid)
		}

		return
	}

	return
}

func HandleAdmin(c *irc.IConn, m *irc.Message) (abort bool) {
	matches := adminRE.FindStringSubmatch(m.Trailing)
	if len(matches) == 0 {
		return
	}

	var savestate bool
	abort = true

	command := matches[1]
	factoidkey := strings.ToLower(matches[2])
	newfactoidkey := strings.ToLower(matches[3])
	factoid := matches[3]
	if len(matches[4]) > 0 {
		factoid = matches[3] + matches[4]
	}

	switch command {
	case "add":
		fallthrough
	case "mod":
		state.Lock()
		defer state.Unlock()

		factoids[factoidkey] = factoid
		savestate = true
		c.Notice(m, "Added/Modified successfully")

	case "del":
		state.Lock()
		defer state.Unlock()

	restartdelete:
		if _, ok := factoids[factoidkey]; ok {
			delete(factoids, factoidkey)
			c.Notice(m, "Deleted successfully")
			// clean up the aliases too
			for k, v := range aliases {
				if v == factoidkey {
					delete(aliases, k)
				}
			}
		} else if factoidkey, ok = aliases[factoidkey]; ok {
			c.Notice(m, "Found an alias, deleting the original factoid")
			goto restartdelete
		}

		savestate = true

	case "rename":
		if !alphaRE.MatchString(newfactoidkey) {
			return
		}
		state.Lock()
		defer state.Unlock()

		if _, ok := factoids[newfactoidkey]; ok {
			c.Notice(m, "Renaming would overwrite, please delete first")
			return
		}
		if _, ok := aliases[newfactoidkey]; ok {
			c.Notice(m, "Renaming would overwrite an alias, please delete first")
			return
		}
		if _, ok := factoids[factoidkey]; ok {
			factoids[newfactoidkey] = factoids[factoidkey]
			delete(factoids, factoidkey)
			// rename the aliases too
			for k, v := range aliases {
				if v == factoidkey {
					aliases[k] = newfactoidkey
				}
			}
			savestate = true
			c.Notice(m, "Renamed successfully")
		} else {
			c.Notice(m, "Not present")
		}

	case "addalias":
		fallthrough
	case "modalias":
		if !alphaRE.MatchString(newfactoidkey) {
			return
		}

		state.Lock()
		defer state.Unlock()

		// newfactoidkey is the factoid we are going to add an alias for
		// if itself is an alias, get the original factoid key, that is what
		// getfactoidByKey does
		_, newfactoidkey, ok := getfactoidByKey(newfactoidkey)
		if ok {
			aliases[factoidkey] = newfactoidkey
			savestate = true
			c.Notice(m, "Added/Modified alias for ", newfactoidkey, " successfully")
		} else {
			c.Notice(m, "No factoid with name ", newfactoidkey, " found")
		}

	case "delalias":
		state.Lock()
		defer state.Unlock()

		if _, ok := aliases[factoidkey]; ok {
			c.Notice(m, "Deleted alias successfully")
			delete(aliases, factoidkey)
			savestate = true
		}

	default:
		abort = false
		return
	}

	if savestate {
		state.Save(false)
		tpl.invalidate()
	}

	return
}
