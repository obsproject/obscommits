package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"time"

	"github.com/sztanpet/obscommits/internal/persist"
)

type Statedata struct {
	Factoids         map[string]string
	Factoidaliases   map[string]string
	Seenrss          map[string]int64
	Seengithubevents map[string]int64
	Admins           map[string]bool
}

var statePath = flag.String("statepath", ".state.dc", "the state file")

func main() {
	flag.Parse()
	s := &Statedata{
		Factoids:         map[string]string{},
		Factoidaliases:   map[string]string{},
		Seenrss:          map[string]int64{},
		Seengithubevents: map[string]int64{},
		Admins:           map[string]bool{},
	}
	save(*statePath, s)
	fmt.Printf("migrating state, values were:\n%#v\n", s)

	save("factoids.state", &struct {
		Factoids map[string]string
		Aliases  map[string]string
		Used     map[string]time.Time
	}{
		Factoids: s.Factoids,
		Aliases:  s.Factoidaliases,
		Used:     map[string]time.Time{},
	})

	admins := map[string]struct{}{}
	for admin := range s.Admins {
		admins[admin] = struct{}{}
	}
	save("admins.state", &admins)

	rss := map[[16]byte]int64{}
	for h, t := range s.Seenrss {
		tempHash, err := base64.StdEncoding.DecodeString(h)
		if err != nil {
			fmt.Printf("could not decode string %v\n", h)
			continue
		}
		var hash [16]byte
		copy(hash[:], tempHash)

		rss[hash] = t
	}
	save("rss.state", &rss)
}

func save(path string, data interface{}) {
	_, err := persist.New(path, data)
	if err != nil {
		panic(err.Error())
	}
}
