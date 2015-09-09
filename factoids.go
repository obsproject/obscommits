package main

import (
	"bytes"
	"html/template"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sztanpet/obscommits/internal/debug"
	"github.com/sztanpet/obscommits/internal/persist"
	"golang.org/x/net/context"
)

const (
	White      = "0"
	Black      = "1"
	DarkBlue   = "2"
	DarkGreen  = "3"
	Red        = "4"
	DarkRed    = "5"
	DarkViolet = "6"
	Orange     = "7"
	Yellow     = "8"
	LightGreen = "9"
	Cyan       = "10"
	LightCyan  = "11"
	Blue       = "12"
	Violet     = "13"
	DarkGray   = "14"
	LightGray  = "15"
)

var colors = map[string]string{
	White:      "#ffffff",
	Black:      "#000000",
	DarkBlue:   "#3636B2",
	DarkGreen:  "#2A8C2A",
	Red:        "#C33B3B",
	DarkRed:    "#C73232",
	DarkViolet: "#80267F",
	Orange:     "#66361F",
	Yellow:     "#D9A641",
	LightGreen: "#3DCC3D",
	Cyan:       "#1A5555",
	LightCyan:  "#2F8C74",
	Blue:       "#4545E6",
	Violet:     "#B037B0",
	DarkGray:   "#4C4C4C",
	LightGray:  "#959595",
}

const (
	Bold          = "\x02"
	Color         = "\x03"
	Italic        = "\x09"
	StrikeThrough = "\x13"
	Reset         = "\x0f"
	Underline     = "\x15"
	Underline2    = "\x1f"
	Reverse       = "\x16"
)

var tags = map[string][]string{
	Bold:          {"<b>", "</b>"},
	Italic:        {"<i>", "</i>"},
	StrikeThrough: {"<strike>", "</strike>"},
	Underline:     {"<u>", "</u>"},
	Reverse:       {`<span class="reverse">`, "</span>"},
}

var (
	urlre     = regexp.MustCompile(`https?://[^ ]+\.[^ ]+`)
	controlre = regexp.MustCompile("([\x02\x03\x09\x13\x0f\x15\x1f\x16])(?:(\\d+)?(?:,(\\d+))?)?")
	factoids  = map[string]string{}
	aliases   = map[string]string{}
	used      = map[string]time.Time{}
	state     *persist.State
)

func Init(ctx context.Context) context.Context {
	s := struct {
		Factoids map[string]string
		Aliases  map[string]string
		Used     map[string]time.Time
	}{
		Factoids: factoids,
		Aliases:  aliases,
		Used:     used,
	}

	var err error
	state, err = persist.New("factoids.state", &s)
	if err != nil {
		d.F(err.Error())
	}

	return ctx
}

type Factoid struct {
	Name    string
	Text    string
	Aliases []string
}

type Factoids []*Factoid

func (f Factoids) Len() int           { return len(f) }
func (f Factoids) Less(i, j int) bool { return f[i].Name < f[j].Name }
func (f Factoids) Swap(i, j int)      { f[i], f[j] = f[j], f[i] }

type Factoidtpl struct {
	basetpl  *template.Template
	tplmtime time.Time
	tpllen   int
	tplout   []byte
	valid    bool
	sync.Mutex
}

func (f *Factoidtpl) init() {
	f.Lock()
	defer f.Unlock()

	f.basetpl = template.New("main").Funcs(template.FuncMap{
		"linkify": func(s string) template.HTML {
			s = template.HTMLEscapeString(s)
			matches := urlre.FindAllString(s, -1)
			b := bytes.NewBuffer(nil)
			for _, url := range matches {
				b.Reset()
				b.WriteString(`<a target="_blank" href="`)
				b.WriteString(url)
				b.WriteString(`">`)
				b.WriteString(url)
				b.WriteString(`</a>`)
				s = strings.Replace(s, url, b.String(), -1)
			}

			return template.HTML(s)
		},
		"ircize": func(html template.HTML) template.HTML {
			s := string(html)
			// the state that signals what controlcode was started, it is an array
			// because control codes can be stacked and we do not want unclosed tags
			state := map[string][]string{}
			b := bytes.NewBuffer(nil)

			s = controlre.ReplaceAllStringFunc(s, func(m string) string {
				match := controlre.FindStringSubmatch(m)
				controlcode := match[1]
				firstarg := match[2]
				secondarg := match[3]

				// normalize the two underline control codes into one
				if controlcode == Underline2 {
					controlcode = Underline
				}

				// just a controlcode without arguments, if there was one before,
				// this closes it
				if closetags, ok := state[controlcode]; firstarg == "" && ok {
					m = strings.Join(closetags, "")
					delete(state, controlcode)
					return m
				}

				switch controlcode {
				case Bold:
					fallthrough
				case Italic:
					fallthrough
				case StrikeThrough:
					fallthrough
				case Underline:
					fallthrough
				case Reverse:
					closetags := state[controlcode]
					closetags = append(closetags, tags[controlcode][1])
					state[controlcode] = closetags
					return tags[controlcode][0]
				case Color:
					// if there was no previous color started and this one has no
					// arguments than just strip it because its invalid
					if firstarg == "" {
						return ""
					}

					b.Reset()
					// have to close previous span tag if embedded
					if len(state[controlcode]) > 0 {
						b.WriteString(state[controlcode][0])
					} else {
						closetags := state[controlcode]
						closetags = append(closetags, "</span>")
						state[controlcode] = closetags
					}

					b.WriteString(`<span style="color: `)
					b.WriteString(colors[firstarg])
					if secondarg != "" {
						b.WriteString("; background-color: ")
						b.WriteString(colors[secondarg])
					}
					b.WriteString(`">`)
					return b.String()
				case Reset:
					b.Reset()
					for k, a := range state {
						for _, v := range a {
							b.WriteString(v)
						}
						delete(state, k)
					}
					return b.String()
				}

				return m
			})

			// there were unclosed tags, so close them by pasting them to the end of
			// the string
			if len(state) > 0 {
				b.Reset()
				b.WriteString(s)
				for _, a := range state {
					for _, v := range a {
						b.WriteString(v)
					}
				}
				s = b.String()
			}

			return template.HTML(s)
		},
	})

	go (func() {
		t := time.NewTicker(30 * time.Second)
		for {
			<-t.C
			f.checkTemplateChanged()
		}
	})()

}

func (f *Factoidtpl) invalidate() {
	f.Lock()
	defer f.Unlock()
	f.valid = false
}

func (f *Factoidtpl) execute(w http.ResponseWriter) {
	f.Lock()
	defer f.Unlock()

	w.Write(f.tplout[0:f.tpllen])
}

func (f *Factoidtpl) ensureFreshness() {
	f.Lock()
	defer f.Unlock()
	if f.valid {
		return
	}

	var err error
	tpl, _ := f.basetpl.Clone()
	tpl, err = tpl.ParseFiles("factoid.tpl")
	if err != nil {
		d.D("failed parsing file", err)
	}

	data := f.getFactoids()

	b := bytes.NewBuffer(nil)
	tpl.ExecuteTemplate(b, "factoid.tpl", data)
	f.tpllen = b.Len()
	if cap(f.tplout) < f.tpllen {
		f.tplout = make([]byte, f.tpllen)
	}
	io.ReadFull(b, f.tplout)

	f.valid = true
}

func (f *Factoidtpl) getFactoids() []*Factoid {
	state.Lock()
	defer state.Unlock()

	a := make(map[string][]string)
	for alias, factoid := range aliases {
		a[factoid] = append(a[factoid], alias)
	}

	for _, v := range a {
		sort.Strings(v)
	}

	fs := make([]*Factoid, 0, len(factoids))
	for name, text := range factoids {
		fs = append(fs, &Factoid{
			Name:    name,
			Text:    text,
			Aliases: a[name],
		})
	}

	sort.Sort(Factoids(fs))
	return fs
}

func (f *Factoidtpl) checkTemplateChanged() {
	f.Lock()
	defer f.Unlock()

	if !f.valid {
		return
	}

	info, err := os.Stat("factoid.tpl")
	if err != nil {
		d.D("Error stating factoid.tpl", err)
		return
	}

	if !info.ModTime().Equal(f.tplmtime) {
		f.valid = false
	}
}

var factoidtpl = Factoidtpl{}

func initFactoids(hookpath string) {
	factoidtpl.init()

	http.HandleFunc(hookpath, handleFactoidRequest)
}

func handleFactoidRequest(w http.ResponseWriter, r *http.Request) {
	factoidtpl.ensureFreshness()
	factoidtpl.execute(w)
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
func getFactoidByKey(factoidkey string) (factoid, key string, ok bool) {
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
	state.Lock()
	defer state.Unlock()
	if factoid, factoidkey, ok := getFactoidByKey(factoidkey); ok {
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

func tryHandleAdminFactoid(target, nick string, parts []string) (abort bool) {
	var newfactoidkey string
	var factoid string
	var savestate bool
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
		state.Lock()
		defer state.Unlock()

		factoids[factoidkey] = factoid
		savestate = true
		srv.notice(nick, "Added/Modified successfully")

	case "del":
		state.Lock()
		defer state.Unlock()

	restartdelete:
		if _, ok := factoids[factoidkey]; ok {
			delete(factoids, factoidkey)
			srv.notice(nick, "Deleted successfully")
			// clean up the aliases too
			for k, v := range aliases {
				if v == factoidkey {
					delete(aliases, k)
				}
			}
		} else if factoidkey, ok = aliases[factoidkey]; ok {
			srv.notice(nick, "Found an alias, deleting the original factoid")
			goto restartdelete
		}

		savestate = true

	case "rename":
		if !isalpha.MatchString(newfactoidkey) {
			return
		}
		state.Lock()
		defer state.Unlock()

		if _, ok := factoids[newfactoidkey]; ok {
			srv.notice(nick, "Renaming would overwrite, please delete first")
			return
		}
		if _, ok := aliases[newfactoidkey]; ok {
			srv.notice(nick, "Renaming would overwrite an alias, please delete first")
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
		state.Lock()
		defer state.Unlock()

		// newfactoidkey is the factoid we are going to add an alias for
		// if itself is an alias, get the original factoid key, that is what
		// getFactoidByKey does
		_, newfactoidkey, ok := getFactoidByKey(newfactoidkey)
		if ok {
			aliases[factoidkey] = newfactoidkey
			savestate = true
			srv.notice(nick, "Added/Modified alias for ", newfactoidkey, " successfully")
		} else {
			srv.notice(nick, "No factoid with name ", newfactoidkey, " found")
		}

	case "delalias":
		state.Lock()
		defer state.Unlock()

		if _, ok := aliases[factoidkey]; ok {
			srv.notice(nick, "Deleted alias successfully")
			delete(aliases, factoidkey)
			savestate = true
		}

	default:
		abort = false
		return
	}

	if savestate {
		state.Save()
		factoidtpl.invalidate()
	}

	return
}
