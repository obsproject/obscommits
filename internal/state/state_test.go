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
	"os"
	"testing"
)

const testFileName = "teststate.tmp"

func TestSanity(t *testing.T) {
	m := map[string]string{
		"foo": "bar",
	}

	_ = os.Remove(testFileName)
	defer os.Remove(testFileName)

	s, err := New(testFileName, &m)
	if err != nil || len(m) != 1 || m["foo"] != "bar" {
		t.Fatalf("unexpected map %#v, err %v", m, err)
	}

	m["bar"] = "foo"
	m["foo"] = "bar2"
	err = s.Save()
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}

	// the file exists, so our map should be overwritten by the existing
	// values, plus the non-existing value should be retained
	m = map[string]string{
		"foo": "bar",
		"qwe": "asd",
	}
	s, err = New(testFileName, &m)
	if err != nil || len(m) != 3 || m["foo"] != "bar2" || m["bar"] != "foo" || m["qwe"] != "asd" {
		t.Fatalf("unexpected map %#v, err %v", m, err)
	}
}
