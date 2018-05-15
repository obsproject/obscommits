package main

import (
	"archive/zip"
	"encoding/base64"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/obsproject/obscommits/internal/analyzer"
	"github.com/obsproject/obscommits/internal/config"
	"github.com/obsproject/obscommits/internal/debug"
	"github.com/obsproject/obscommits/internal/factoids"
	"github.com/obsproject/obscommits/internal/persist"
	"github.com/sztanpet/sirc"
	"golang.org/x/net/context"
	"gopkg.in/sorcix/irc.v1"
)

var (
	adminState *persist.State
	admins     map[string]struct{}
	adminRE    = regexp.MustCompile(`^\.(addadmin|deladmin|raw|downloadstate)(?:\s+(.*))?$`)
	zippathRE  = regexp.MustCompile(`^[A-Za-z0-9]+$`)
	downloads  = stateDownload{
		m: map[string]struct{}{},
	}
)

type stateDownload struct {
	mu sync.Mutex
	m  map[string]struct{}
}

func (s *stateDownload) pathValid(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.m[path]; ok {
		return true
	}

	return false
}

func (s *stateDownload) addPath(path string) {
	s.mu.Lock()
	s.m[path] = struct{}{}
	s.mu.Unlock()
}

func (s *stateDownload) delPath(path string) {
	s.mu.Lock()
	delete(s.m, path)
	s.mu.Unlock()
}

func initIRC(ctx context.Context) context.Context {
	adminRE.Longest()

	// handle state downloading
	http.HandleFunc("/state/", func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Path) < len("/state/")+10 {
			http.NotFound(w, r)
			return
		}
		zippath := r.URL.Path[len("/state/"):]
		if zippathRE.MatchString(zippath) == false || downloads.pathValid(zippath) == false {
			http.NotFound(w, r)
			return
		}

		f, err := os.Open(zippath)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// cleanup after
		defer (func() {
			f.Close()
			downloads.delPath(zippath)
			_ = os.Remove(zippath)
		})()

		w.Header().Add("Content-Disposition", "attachment; filename=\"obscommits-backup.zip\"")
		http.ServeContent(w, r, "obscommits-backup.zip", time.Now(), f)
	})

	var err error
	adminState, err = persist.New("admins.state", &map[string]struct{}{
		"melkor":                       struct{}{},
		"melkor.lan":                   struct{}{},
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

	admins = *adminState.Get().(*map[string]struct{})

	tcfg := config.FromContext(ctx)
	sirc.DebuggingEnabled = tcfg.Debug.Debug
	cfg := sirc.Config{
		Addr:     tcfg.IRC.Addr,
		Nick:     tcfg.IRC.Nick,
		Password: tcfg.IRC.Password,
		RealName: tcfg.Website.BaseURL,
	}
	c := sirc.Init(cfg, func(c *sirc.IConn, m *irc.Message) bool {
		return handleIRC(ctx, c, m)
	})

	return c.ToContext(ctx)
}

func handleIRC(ctx context.Context, c *sirc.IConn, m *irc.Message) bool {
	if m.Command == irc.RPL_WELCOME {
		cfg := config.FromContext(ctx).IRC
		for _, ch := range cfg.Channels {
			c.Write(&irc.Message{Command: irc.JOIN, Params: []string{ch}})
		}

		return false
	}

	if m.Command != irc.PRIVMSG {
		return false
	}

	if factoids.Handle(c, m) == true {
		return true
	}
	if analyzer.Handle(c, m) == true {
		return true
	}

	if m.Prefix != nil && len(m.Prefix.Host) > 0 {
		adminState.Lock()
		_, admin := admins[m.Prefix.Host]
		adminState.Unlock()
		if !admin {
			return true
		}
		if factoids.HandleAdmin(c, m) {
			return true
		}

		if handleAdmin(ctx, c, m) {
			return true
		}
	}

	return false
}

func handleAdmin(ctx context.Context, c *sirc.IConn, m *irc.Message) bool {
	matches := adminRE.FindStringSubmatch(m.Trailing)
	d.P(matches, m)
	if len(matches) == 0 {
		return false
	}
	adminState.Lock()
	// lifo defer order
	defer adminState.Save()
	defer adminState.Unlock()

	host := strings.TrimSpace(matches[2])
	switch matches[1] {
	case "addadmin":
		admins[host] = struct{}{}
		c.Notice(m, "Added host successfully")
	case "deladmin":
		delete(admins, host)
		c.Notice(m, "Removed host successfully")
	case "raw":
		nm := irc.ParseMessage(matches[2])
		if nm == nil {
			c.Notice(m, "Could not parse, are you sure you know the irc protocol?")
		} else {
			go c.Write(nm)
		}
	case "downloadstate":
		// generate random filename
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		u := make([]byte, 32)
		_, _ = r.Read(u)

		// just save it into the current working directory for now
		zippath := strings.Map(func(r rune) rune {
			if strings.IndexRune("+/=", r) < 0 {
				return r
			}
			return -1
		}, base64.StdEncoding.EncodeToString(u))

		// currently the state is contained in these files
		paths := []string{"admins.state", "factoids.state", "rss.state", "settings.cfg"}

		err := generateZip(zippath, paths)
		if err != nil {
			c.Notice(m, "Error while generating zip: "+err.Error())
			return true
		}

		downloads.addPath(zippath)
		go (func() {
			<-time.After(5 * time.Minute)
			downloads.delPath(zippath)
			_ = os.Remove(zippath)
		})()

		url := config.FromContext(ctx).Website.BaseURL + "/state/" + zippath
		c.Notice(m, "Your one-time use URL (expiring in 5 minutes) is: "+url)
	}

	return true
}

// based on https://golangcode.com/create-zip-files-in-go/
func generateZip(zippath string, paths []string) error {
	zf, err := os.Create(zippath)
	if err != nil {
		return err
	}

	defer zf.Close()

	zw := zip.NewWriter(zf)
	defer zw.Close()

	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return err
		}

		defer f.Close()

		i, err := f.Stat()
		if err != nil {
			return err
		}

		h, err := zip.FileInfoHeader(i)
		if err != nil {
			return err
		}

		h.Method = zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}

		// from the file into the zip
		_, err = io.Copy(w, f)
		if err != nil {
			return err
		}
	}

	return nil
}
