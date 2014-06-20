#!/bin/bash
go build $1 -o obscommits main.go irc.go github.go debug.go rss.go analyzer.go factoids.go state.go templates.go
