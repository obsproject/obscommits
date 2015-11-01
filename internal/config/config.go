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

package config

import (
	"flag"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/naoina/toml"
	"golang.org/x/net/context"
)

type Website struct {
	Addr string `toml:"addr"`
}

type Analyzer struct {
	URL string `toml:"url"`
}

type Factoids struct {
	HookPath string `toml:"hookpath"`
	TplPath  string `toml:"tplpath"`
}

type Debug struct {
	Debug   bool   `toml:"debug"`
	Logfile string `toml:"logfile"`
}

type Github struct {
	HookPath     string `toml:"hookpath"`
	AnnounceChan string `toml:"announcechan"`
}

type IRC struct {
	Addr     string   `toml:"addr"`
	Ident    string   `toml:"ident"`
	Nick     string   `toml:"nick"`
	UserName string   `toml:"username"`
	Password string   `toml:"password"`
	Channels []string `toml:"channels"`
}

type RSS struct {
	ForumURL   string `toml:"forumurl"`
	ForumChan  string `toml:"forumchan"`
	MantisURL  string `toml:"mantisurl"`
	MantisChan string `toml:"mantischan"`
}

type AppConfig struct {
	Website
	Debug
	Factoids
	Analyzer
	Github
	IRC `toml:"irc"`
	RSS `toml:"rss"`
}

var settingsFile *string

const sampleconf = `[website]
addr=":80"

[debug]
debug=false
logfile="logs/debug.txt"

[factoids]
hookpath="/"

[analyzer]
url="http://obsproject.com/analyzer?"

[github]
hookpath="somethingrandom"
announcechan="#obs-dev"

[irc]
addr="irc.freenode.net:6667"
ident="obscommits"
username="http://obscommits.sztanpet.net/"
nick="obscommits"
password=""
channels=["#obs-dev", "#obscommits"]

[rss]
forumurl="https://obsproject.com/forum/list/-/index.rss?order=post_date"
forumchan="#obscommits"
mantisurl="https://obsproject.com/mantis/issues_rss.php?"
mantischan="#obs-dev"
`

var contextKey *int

func init() {
	contextKey = new(int)
}

func Init(ctx context.Context) context.Context {
	settingsFile = flag.String("config", "settings.cfg", `path to the config file, it it doesn't exist it will
			be created with default values`)
	flag.Parse()

	f, err := os.OpenFile(*settingsFile, os.O_CREATE|os.O_RDWR, 0660)
	if err != nil {
		panic("Could not open " + *settingsFile + " err: " + err.Error())
	}
	defer f.Close()

	// empty? initialize it
	if info, err := f.Stat(); err == nil && info.Size() == 0 {
		io.WriteString(f, sampleconf)
		f.Seek(0, 0)
	}

	cfg := &AppConfig{}
	if err := ReadConfig(f, cfg); err != nil {
		panic("Failed to parse config file, err: " + err.Error())
	}

	return context.WithValue(ctx, contextKey, cfg)
}

func ReadConfig(r io.Reader, d interface{}) error {
	dec := toml.NewDecoder(r)
	return dec.Decode(d)
}

func WriteConfig(w io.Writer, d interface{}) error {
	enc := toml.NewEncoder(w)
	return enc.Encode(d)
}

func Save(ctx context.Context) error {
	return SafeSave(*settingsFile, *FromContext(ctx))
}

func SafeSave(file string, data interface{}) error {
	dir, err := filepath.Abs(filepath.Dir(file))
	if err != nil {
		return err
	}

	f, err := ioutil.TempFile(dir, "tmpconf-")
	if err != nil {
		return err
	}

	err = WriteConfig(f, data)
	if err != nil {
		return err
	}
	_ = f.Close()

	return os.Rename(f.Name(), file)
}

func FromContext(ctx context.Context) *AppConfig {
	cfg, _ := ctx.Value(contextKey).(*AppConfig)
	return cfg
}
