package main

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/gob"
	"io/ioutil"
	"sort"
	"sync"
	"time"
)

type sortableInt64 []int64

func (a sortableInt64) Len() int           { return len(a) }
func (a sortableInt64) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a sortableInt64) Less(i, j int) bool { return a[i] < a[j] }

type State struct {
	data Statedata
	sync.Mutex
}

type Statedata struct {
	Factoids         map[string]string
	Factoidaliases   map[string]string
	Seenrss          map[string]int64
	Seengithubevents map[string]int64
	Admins           map[string]bool
}

func (s *State) init() {
	s.Lock()
	defer s.Unlock()

	s.load()

	if s.data.Factoids == nil {
		s.data.Factoids = make(map[string]string)
	}

	if s.data.Factoidaliases == nil {
		s.data.Factoidaliases = make(map[string]string)
	}

	if s.data.Seenrss == nil {
		s.data.Seenrss = make(map[string]int64)
	}

	if s.data.Seengithubevents == nil {
		s.data.Seengithubevents = make(map[string]int64)
	}

	if s.data.Admins == nil || !s.data.Admins["melkor"] {
		s.data.Admins = map[string]bool{
			"melkor":                       true,
			"sztanpet.users.quakenet.org":  true,
			"R1CH.users.quakenet.org":      true,
			"Jim.users.quakenet.org":       true,
			"Warchamp7.users.quakenet.org": true,
			"hwd.users.quakenet.org":       true,
			"paibox.users.quakenet.org":    true,
			"ThoNohT.users.quakenet.org":   true,
			"dodgepong.users.quakenet.org": true,
			"Sapiens.users.quakenet.org":   true,
		}
	}

	s.save()
}

// expects to be called with locks held
func (s *State) save() {
	b := bytes.NewBuffer(nil)
	enc := gob.NewEncoder(b)
	err := enc.Encode(s.data)
	if err != nil {
		D("Error encoding s:", err)
	}
	err = ioutil.WriteFile(".state.dc", b.Bytes(), 0600)
	if err != nil {
		D("Error with writing out s file:", err)
	}
}

// expects to be called with locks held
func (s *State) load() {

	contents, err := ioutil.ReadFile(".state.dc")
	if err != nil {
		D("Error while reading from state file")
		return
	}

	buff := bytes.NewBuffer(contents)
	dec := gob.NewDecoder(buff)
	err = dec.Decode(&s.data)

	if err != nil {
		D("Error decoding state, initializing", err)
	}

}

func (s *State) addRssHash(id string) (added bool) {
	s.Lock()

	hash := s.getHash(id)
	if _, ok := s.data.Seenrss[hash]; !ok {
		s.data.Seenrss[hash] = time.Now().UTC().UnixNano()
		added = true
	}

	if len(s.data.Seenrss) > 2000 {
		s.gcItems(s.data.Seenrss, 2000)
	}

	s.save()
	s.Unlock()

	return added
}

func (s *State) addGithubEvent(id string) (added bool) {
	s.Lock()

	hash := s.getHash(id)
	if _, ok := s.data.Seengithubevents[hash]; !ok {
		added = true
		s.data.Seengithubevents[hash] = time.Now().UTC().UnixNano()
	}

	if len(s.data.Seengithubevents) > 30 {
		s.gcItems(s.data.Seengithubevents, 30)
	}

	s.save()
	s.Unlock()

	return added
}

// expects to be called with locks held
// deletes the oldest elements from the map leaving numItems behind
func (s *State) gcItems(m map[string]int64, numItems int) {

	timestamps := make(sortableInt64, 0, len(m))
	for _, ts := range m {
		timestamps = append(timestamps, ts)
	}

	sort.Sort(timestamps)
	timestamps = timestamps[:len(m)-numItems]
	for key, value := range m {

		for _, ts := range timestamps {
			if value == ts {
				delete(m, key)
				break
			}
		}

	}
}

func (s *State) getHash(data string) string {
	hash := md5.Sum([]byte(data))
	return base64.StdEncoding.EncodeToString(hash[:])
}

func (s *State) isAdmin(host string) bool {
	s.Lock()
	defer s.Unlock()
	_, ok := s.data.Admins[host]
	return ok
}
func (s *State) addAdmin(host string) {
	s.Lock()
	s.data.Admins[host] = true
	s.save()
	s.Unlock()
}
func (s *State) delAdmin(host string) {
	s.Lock()
	delete(s.data.Admins, host)
	s.save()
	s.Unlock()
}
