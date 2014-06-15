package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"text/template"
)

type GHAuthor struct {
	Username string
}
type GHCommit struct {
	Author  GHAuthor
	Url     string
	Message string
	Id      string
}
type GHRepo struct {
	Name string
	Url  string
}
type GHJson struct {
	Ref        string
	Commits    []GHCommit
	Repository GHRepo
}

type Commit struct {
	Author  string // commits[0].author.username
	Url     string // commits[0].url
	Message string // commits[0].message
	ID      string // commits[0].id
	Repo    string // repository.name
	Repourl string // repository.url
	Branch  string // .ref the part after refs/heads/
}

func initGithub(hookpath string) {
	tmpllock.Lock()
	tmpl = template.Must(tmpl.Parse(`{{define "git"}}[{{.Repo}}|{{.Author}}] {{truncate .Message 200 "..."}} {{.Repourl}}/commit/{{truncate .ID 7 ""}}{{end}}`))
	tmpllock.Unlock()

	http.HandleFunc(hookpath, func(w http.ResponseWriter, r *http.Request) {
		payload := r.FormValue("payload")
		if len(payload) == 0 {
			return
		}

		D("received payload:\n", payload)

		var data GHJson
		err := json.Unmarshal([]byte(payload), &data)
		if err != nil {
			P("Error unmarshaling json:", err, "payload was: ", payload)
			return
		}

		pos := strings.LastIndex(data.Ref, "/") + 1
		branch := data.Ref[pos:]
		commits := make([]string, 0, len(data.Commits))
		repo := data.Repository.Name
		repourl := data.Repository.Url
		b := bytes.NewBuffer(nil)

		tmpllock.Lock()
		defer tmpllock.Unlock()
		for _, v := range data.Commits {
			firstline := strings.TrimSpace(v.Message)
			pos = strings.Index(firstline, "\n")
			if pos > 0 {
				firstline = strings.TrimSpace(firstline[:pos])
			}

			if branch != "master" {
				continue // we don't care about anything but the master branch :(
			}

			b.Reset()
			tmpl.ExecuteTemplate(b, "git", &Commit{
				Author:  v.Author.Username,
				Url:     v.Url,
				Message: firstline,
				ID:      v.Id,
				Repo:    repo,
				Repourl: repourl,
				Branch:  branch,
			})
			commits = append(commits, b.String())
		}

		go srv.handleLines("#obs-dev", commits, true)
	})

}

/*
{
   "after":"1481a2de7b2a7d02428ad93446ab166be7793fbb",
   "before":"17c497ccc7cca9c2f735aa07e9e3813060ce9a6a",
   "commits":[
      {
         "added":[

         ],
         "author":{
            "email":"lolwut@noway.biz",
            "name":"Garen Torikian",
            "username":"octokitty"
         },
         "committer":{
            "email":"lolwut@noway.biz",
            "name":"Garen Torikian",
            "username":"octokitty"
         },
         "distinct":true,
         "id":"c441029cf673f84c8b7db52d0a5944ee5c52ff89",
         "message":"Test",
         "modified":[
            "README.md"
         ],
         "removed":[

         ],
         "timestamp":"2013-02-22T13:50:07-08:00",
         "url":"https://github.com/octokitty/testing/commit/c441029cf673f84c8b7db52d0a5944ee5c52ff89"
      },
      {
         "added":[

         ],
         "author":{
            "email":"lolwut@noway.biz",
            "name":"Garen Torikian",
            "username":"octokitty"
         },
         "committer":{
            "email":"lolwut@noway.biz",
            "name":"Garen Torikian",
            "username":"octokitty"
         },
         "distinct":true,
         "id":"36c5f2243ed24de58284a96f2a643bed8c028658",
         "message":"This is me testing the windows client.",
         "modified":[
            "README.md"
         ],
         "removed":[

         ],
         "timestamp":"2013-02-22T14:07:13-08:00",
         "url":"https://github.com/octokitty/testing/commit/36c5f2243ed24de58284a96f2a643bed8c028658"
      },
      {
         "added":[
            "words/madame-bovary.txt"
         ],
         "author":{
            "email":"lolwut@noway.biz",
            "name":"Garen Torikian",
            "username":"octokitty"
         },
         "committer":{
            "email":"lolwut@noway.biz",
            "name":"Garen Torikian",
            "username":"octokitty"
         },
         "distinct":true,
         "id":"1481a2de7b2a7d02428ad93446ab166be7793fbb",
         "message":"Rename madame-bovary.txt to words/madame-bovary.txt",
         "modified":[

         ],
         "removed":[
            "madame-bovary.txt"
         ],
         "timestamp":"2013-03-12T08:14:29-07:00",
         "url":"https://github.com/octokitty/testing/commit/1481a2de7b2a7d02428ad93446ab166be7793fbb"
      }
   ],
   "compare":"https://github.com/octokitty/testing/compare/17c497ccc7cc...1481a2de7b2a",
   "created":false,
   "deleted":false,
   "forced":false,
   "head_commit":{
      "added":[
         "words/madame-bovary.txt"
      ],
      "author":{
         "email":"lolwut@noway.biz",
         "name":"Garen Torikian",
         "username":"octokitty"
      },
      "committer":{
         "email":"lolwut@noway.biz",
         "name":"Garen Torikian",
         "username":"octokitty"
      },
      "distinct":true,
      "id":"1481a2de7b2a7d02428ad93446ab166be7793fbb",
      "message":"Rename madame-bovary.txt to words/madame-bovary.txt",
      "modified":[

      ],
      "removed":[
         "madame-bovary.txt"
      ],
      "timestamp":"2013-03-12T08:14:29-07:00",
      "url":"https://github.com/octokitty/testing/commit/1481a2de7b2a7d02428ad93446ab166be7793fbb"
   },
   "pusher":{
      "email":"lolwut@noway.biz",
      "name":"Garen Torikian"
   },
   "ref":"refs/heads/master",
   "repository":{
      "created_at":1332977768,
      "description":"",
      "fork":false,
      "forks":0,
      "has_downloads":true,
      "has_issues":true,
      "has_wiki":true,
      "homepage":"",
      "id":3860742,
      "language":"Ruby",
      "master_branch":"master",
      "name":"testing",
      "open_issues":2,
      "owner":{
         "email":"lolwut@noway.biz",
         "name":"octokitty"
      },
      "private":false,
      "pushed_at":1363295520,
      "size":2156,
      "stargazers":1,
      "url":"https://github.com/octokitty/testing",
      "watchers":1
   }
}
*/
