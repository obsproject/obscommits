package factoids

import (
	"bytes"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/sztanpet/obscommits/internal/debug"
	"mvdan.cc/xurls"
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

type cache struct {
	mu    sync.RWMutex
	t     *template.Template
	cache []byte
	valid bool
}

var tpl = &cache{}

func (c *cache) init() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.t = template.New("main").Funcs(template.FuncMap{
		"linkify": func(s string) template.HTML {
			// find urls, replace them with placeholders
			seed := rand.Int()
			placeholder := fmt.Sprintf("|%d-%%d-%d|", seed, seed)
			matches := xurls.Strict().FindAllString(s, -1)
			for ix, url := range matches {
				matches[ix] = template.HTMLEscapeString(url)
				s = strings.Replace(s, url, fmt.Sprintf(placeholder, ix), -1)
			}

			// escape unsafe html
			s = template.HTMLEscapeString(s)

			// replace placeholders with html
			b := bytes.NewBuffer(nil)
			for ix, url := range matches {
				b.Reset()
				b.WriteString(`<a target="_blank" href="`)
				b.WriteString(url)
				b.WriteString(`">`)
				b.WriteString(url)
				b.WriteString(`</a>`)
				s = strings.Replace(s, fmt.Sprintf(placeholder, ix), b.String(), -1)
			}

			return template.HTML(s)
		},
		"ircize": ircToHTML,
	})

	tpl, err := c.t.ParseFiles("factoid.tpl")
	if err != nil {
		d.F("Unable to parse factoid.tpl, err:", err)
	}

	c.t = tpl
}

func (c *cache) invalidate() {
	c.mu.Lock()
	c.valid = false
	c.mu.Unlock()
}

func (c *cache) execute(w http.ResponseWriter) {
	c.mu.RLock()
	w.Write(c.cache)
	c.mu.RUnlock()
}

func (c *cache) render() {
	c.mu.RLock()
	if c.valid {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()

	c.mu.Lock()
	b := bytes.NewBuffer(nil)
	c.t.ExecuteTemplate(b, "factoid.tpl", c.sortFactoids())
	c.cache = b.Bytes()
	c.valid = true
	c.mu.Unlock()
}

func (c *cache) sortFactoids() []factoid {
	state.Lock()
	defer state.Unlock()

	a := make(map[string][]string)
	for alias, factoid := range s.Aliases {
		a[factoid] = append(a[factoid], alias)
	}

	for _, v := range a {
		sort.Strings(v)
	}

	fs := make([]factoid, 0, len(s.Factoids))
	for name, text := range s.Factoids {
		fs = append(fs, factoid{
			Name:    name,
			Text:    text,
			Aliases: a[name],
		})
	}

	sort.Sort(factoidSlice(fs))
	return fs
}
