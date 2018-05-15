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

// The d package is just a very simple collection of debugging and logging aids
// everything is safe to call from anywhere
package d

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/obsproject/obscommits/internal/config"
	"golang.org/x/net/context"
)

const (
	EnableDebug  = true
	DisableDebug = false
)

var (
	mu               sync.RWMutex
	debuggingEnabled bool
)

// Init initializes the printing of debugging information based on its arg
func Init(ctx context.Context) context.Context {
	cfg := config.FromContext(ctx)

	mu.Lock()
	debuggingEnabled = cfg.Debug.Debug
	mu.Unlock()

	logfile := cfg.Debug.Logfile
	w, err := os.OpenFile(logfile, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
	if err != nil {
		panic(logfile + err.Error())
	}
	mw := io.MultiWriter(os.Stderr, w)
	log.SetOutput(mw)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	return ctx
}

func shouldPrint() bool {
	mu.RLock()
	defer mu.RUnlock()
	return debuggingEnabled
}

func getFormatStr(numargs int, addlineinfo bool) string {
	var formatStr string
	if addlineinfo {
		formatStr = "%+v:%+v \n  "
	} else {
		formatStr = "\n  "
	}
	for i := numargs; i >= 1; i-- {
		formatStr += "|%+v|\n  "
	}
	formatStr += "\n\n"
	return formatStr
}

func logWithCaller(file string, line int, args ...interface{}) {
	pos := strings.LastIndex(file, "obscommits/") + len("obscommits/")
	newargs := make([]interface{}, 0, len(args)+2)
	newargs = append(newargs, file[pos:], line)
	newargs = append(newargs, args...)
	log.Printf(getFormatStr(len(args), true), newargs...)
}

// D prints debug info about its arguments depending on whether debug printing
// is enabled or not
func D(args ...interface{}) {
	if shouldPrint() {
		_, file, line, ok := runtime.Caller(1)
		if ok {
			logWithCaller(file, line, args...)
		} else {
			log.Printf(getFormatStr(len(args), false), args...)
		}
	}
}

// DF prints debug info and allows specifying how many stack frames to skip and
// expects a format string, only prints if debug printing is enabled
func DF(skip int, format string, args ...interface{}) {
	if !shouldPrint() {
		return
	}

	_, file, line, ok := runtime.Caller(skip)
	if ok {
		pos := strings.LastIndex(file, "obscommits/") + len("obscommits/")
		newargs := make([]interface{}, 0, len(args)+2)
		newargs = append(newargs, file[pos:], line)
		newargs = append(newargs, args...)
		log.Printf("%+v:%+v "+format+"\n", newargs...)
	} else {
		log.Printf(format, args...)
	}
}

// P prints debug info about its arguments always
func P(args ...interface{}) {
	_, file, line, ok := runtime.Caller(1)
	if ok {
		logWithCaller(file, line, args...)
	} else {
		log.Printf(getFormatStr(len(args), false), args...)
	}
}

// PF prints debug info and allows specifying how many stack frames to skip and
// expects a format string, always prints
func PF(skip int, format string, args ...interface{}) {
	_, file, line, ok := runtime.Caller(skip)
	if ok {
		pos := strings.LastIndex(file, "obscommits/") + len("obscommits/")
		newargs := make([]interface{}, 0, len(args)+2)
		newargs = append(newargs, file[pos:], line)
		newargs = append(newargs, args...)
		log.Printf("%+v:%+v "+format+"\n", newargs...)
	} else {
		log.Printf(format, args...)
	}
}

// F calls panic with a formatted string based on its arguments
func F(format string, args ...interface{}) {
	panic(fmt.Sprintf(format, args...))
}

// source https://groups.google.com/forum/?fromgroups#!topic/golang-nuts/C24fRw8HDmI
// from David Wright
type ErrorTrace struct {
	trace bytes.Buffer
}

func NewErrorTrace(skip int, args ...interface{}) error {
	buf := bytes.Buffer{}

	formatStr := "\n  "
	for i := len(args); i >= 1; i-- {
		formatStr += "|%+v|\n  "
	}
	formatStr += "\n"

	if len(formatStr) != 0 {
		buf.WriteString(fmt.Sprintf(formatStr, args...))
	}

addtrace:
	pc, file, line, ok := runtime.Caller(skip)
	if ok && skip < 15 { // print a max of 15 lines of trace
		fun := runtime.FuncForPC(pc)
		buf.WriteString(fmt.Sprint(fun.Name(), " -- ", file, ":", line, "\n"))
		skip++
		goto addtrace
	}

	if buf.Len() > 0 {
		return ErrorTrace{trace: buf}
	}

	return errors.New("error generating error")
}

func (et ErrorTrace) Error() string {
	return et.trace.String()
}

// BT prints a backtrace
func BT(args ...interface{}) {
	ts := time.Now().Format("2006-02-01 15:04:05: ")
	println(ts, NewErrorTrace(2, args...).Error())
}

// FBT prints a backtrace and then panics (fatal backtrace)
func FBT(args ...interface{}) {
	ts := time.Now().Format("2006-02-01 15:04:05: ")
	println(ts, NewErrorTrace(2, args...).Error())
	panic("-----")
}
