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

package state

import (
	"encoding/gob"
	"os"
	"sync"
)

type State struct {
	sync.Mutex
	path string
	data interface{}
}

func New(p string, d interface{}) (*State, error) {
	ret := &State{
		path: p,
		data: d,
	}

	// non-atomic :(
	if ret.exists() {
		err := ret.load()
		if err != nil {
			return nil, err
		}
	}

	err := ret.Save()
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func (s *State) Set(d interface{}) {
	s.Lock()
	s.data = d
	s.Unlock()
}

func (s *State) Get() interface{} {
	s.Lock()
	ret := s.data
	s.Unlock()

	return ret
}

func (s *State) load() error {
	s.Lock()
	f, err := os.OpenFile(s.path, os.O_RDONLY, 0600)
	defer s.close(f)

	d := gob.NewDecoder(f)
	err = d.Decode(s.data)
	if err != nil {
		return err
	}

	return nil
}

func (s *State) Save() error {
	s.Lock()
	tmpPath := s.path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	defer s.close(f)

	if err != nil {
		return err
	}

	e := gob.NewEncoder(f)
	err = e.Encode(s.data)
	if err != nil {
		return err
	}

	err = os.Rename(tmpPath, s.path)
	if err != nil {
		return err
	}

	return nil
}

func (s *State) exists() bool {
	s.Lock()
	f, err := os.OpenFile(s.path, os.O_RDONLY, 0600)
	defer s.close(f)

	// os.IsExist returns false when the error is nil, so use IsNotExist
	return !os.IsNotExist(err)
}

func (s *State) close(f *os.File) {
	s.Unlock()
	if f != nil {
		_ = f.Close()
	}
}
