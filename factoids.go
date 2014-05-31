package main

import (
	"sort"
	"strings"
)

// checks if there is a factoid, if there isnt tries to look if its an alias
// and then recurses with the found factoid
func getFactoidByKey(factoidkey string) (factoid, key string, ok bool) {
	key = factoidkey
restart:
	if factoid, ok = state.Factoids[key]; ok {
		return
	}
	key, ok = state.Factoidaliases[key]
	if ok {
		goto restart
	}

	return
}

func tryHandleFactoid(target, message string) (abort bool) {
	if len(message) == 0 || message[0:1] != "!" {
		return
	}

	pos := strings.Index(message, " ")
	if pos < 0 {
		pos = len(message)
	}
	factoidkey := strings.ToLower(message[1:pos])
	if !isalpha.MatchString(factoidkey) {
		return
	}
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
		srv.privmsg(target, strings.Join(factoidlist, ", "))
	} else if factoid, factoidkey, ok := getFactoidByKey(factoidkey); ok {
		if factoidUsedRecently(factoidkey) {
			return
		}
		if pos != len(message) { // there was a postfix
			rest := message[pos+1:]        // skip the space
			pos = strings.Index(rest, " ") // and search for the next space
			if pos > 0 {                   // and only print the first thing delimeted by a space
				rest = rest[0:pos]
			}
			srv.privmsg(target, rest, ": ", factoid)
		} else { // otherwise just print the factoid
			srv.privmsg(target, factoid)
		}
	}

	return true
}

func tryHandleAdminFactoid(target, nick string, parts []string) (abort, savestate bool) {

	var newfactoidkey string
	var factoid string
	abort = true
	command := parts[0]
	factoidkey := strings.ToLower(parts[1])

	if len(parts) >= 3 {
		newfactoidkey = strings.ToLower(parts[2])
		factoid = parts[2]
	}

	if len(parts) == 4 {
		factoid = parts[2] + " " + parts[3]
	}

	switch command {
	case "add":
		fallthrough
	case "mod":
		if len(parts) < 3 {
			return
		}
		state.Factoids[factoidkey] = factoid
		savestate = true
		srv.notice(nick, "Added/Modified successfully")

	case "del":
	restartdelete:
		if _, ok := state.Factoids[factoidkey]; ok {
			delete(state.Factoids, factoidkey)
			srv.notice(nick, "Deleted successfully")
			// clean up the aliases too
			for k, v := range state.Factoidaliases {
				if v == factoidkey {
					delete(state.Factoidaliases, k)
				}
			}
		} else if factoidkey, ok = state.Factoidaliases[factoidkey]; ok {
			srv.notice(nick, "Found an alias, deleting the original factoid")
			goto restartdelete
		}

		savestate = true

	case "rename":
		if !isalpha.MatchString(newfactoidkey) {
			return
		}
		if _, ok := state.Factoids[newfactoidkey]; ok {
			srv.notice(nick, "Renaming would overwrite, please delete first")
			return
		}
		if _, ok := state.Factoidaliases[newfactoidkey]; ok {
			srv.notice(nick, "Renaming would overwrite an alias, please delete first")
			return
		}
		if _, ok := state.Factoids[factoidkey]; ok {
			state.Factoids[newfactoidkey] = state.Factoids[factoidkey]
			delete(state.Factoids, factoidkey)
			// rename the aliases too
			for k, v := range state.Factoidaliases {
				if v == factoidkey {
					state.Factoidaliases[k] = newfactoidkey
				}
			}
			savestate = true
			srv.notice(nick, "Renamed successfully")
		} else {
			srv.notice(nick, "Not present")
		}

	case "addalias":
		fallthrough
	case "modalias":
		if len(parts) < 3 {
			return
		}
		// newfactoidkey is the factoid we are going to add an alias for
		// if itself is an alias, get the original factoid key, that is what
		// getFactoidByKey does
		_, newfactoidkey, ok := getFactoidByKey(newfactoidkey)
		if ok {
			state.Factoidaliases[factoidkey] = newfactoidkey
			savestate = true
			srv.notice(nick, "Added/Modified alias for ", newfactoidkey, " successfully")
		} else {
			srv.notice(nick, "No factoid with name ", newfactoidkey, " found")
		}

	case "delalias":
		if _, ok := state.Factoidaliases[factoidkey]; ok {
			srv.notice(nick, "Deleted alias successfully")
			delete(state.Factoidaliases, factoidkey)
			savestate = true
		}

	default:
		abort = false
		return
	}

	return
}
