package factoids

import (
	"bytes"
	"html/template"
	"regexp"
	"strings"
)

const (
	white      = "0"
	black      = "1"
	darkBlue   = "2"
	darkGreen  = "3"
	red        = "4"
	darkRed    = "5"
	darkViolet = "6"
	orange     = "7"
	yellow     = "8"
	lightGreen = "9"
	cyan       = "10"
	lightCyan  = "11"
	blue       = "12"
	violet     = "13"
	darkGray   = "14"
	lightGray  = "15"
)

var colors = map[string]string{
	white:      "#ffffff",
	black:      "#000000",
	darkBlue:   "#3636B2",
	darkGreen:  "#2A8C2A",
	red:        "#C33B3B",
	darkRed:    "#C73232",
	darkViolet: "#80267F",
	orange:     "#66361F",
	yellow:     "#D9A641",
	lightGreen: "#3DCC3D",
	cyan:       "#1A5555",
	lightCyan:  "#2F8C74",
	blue:       "#4545E6",
	violet:     "#B037B0",
	darkGray:   "#4C4C4C",
	lightGray:  "#959595",
}

const (
	bold          = "\x02"
	color         = "\x03"
	italic        = "\x09"
	strikeThrough = "\x13"
	reset         = "\x0f"
	underline     = "\x15"
	underline2    = "\x1f"
	reverse       = "\x16"
)

var tags = map[string][]string{
	bold:          {"<b>", "</b>"},
	italic:        {"<i>", "</i>"},
	strikeThrough: {"<strike>", "</strike>"},
	underline:     {"<u>", "</u>"},
	reverse:       {`<span class="reverse">`, "</span>"},
}

var controlRE = regexp.MustCompile("([\x02\x03\x09\x13\x0f\x15\x1f\x16])(?:(\\d+)?(?:,(\\d+))?)?")

func init() {
	controlRE.Longest()
}
func ircToHTML(html template.HTML) template.HTML {
	s := string(html)
	// functions as a stack almost, for any given opened tag, there is a
	// record here, so we can close everything properly
	state := map[string][]string{}
	b := bytes.NewBuffer(nil)

	s = controlRE.ReplaceAllStringFunc(s, func(m string) string {
		match := controlRE.FindStringSubmatch(m)
		controlcode := match[1]
		firstarg := match[2]
		secondarg := match[3]

		// normalize the two underline control codes into one
		if controlcode == underline2 {
			controlcode = underline
		}

		// just a controlcode without arguments, if there was one before,
		// this closes it
		if closetags, ok := state[controlcode]; firstarg == "" && ok {
			m = strings.Join(closetags, "")
			delete(state, controlcode)
			return m
		}

		switch controlcode {
		case bold:
			fallthrough
		case italic:
			fallthrough
		case strikeThrough:
			fallthrough
		case underline:
			fallthrough
		case reverse:
			// push the closing tag onto the stack
			closetags := state[controlcode]
			closetags = append(closetags, tags[controlcode][1])
			state[controlcode] = closetags
			return tags[controlcode][0]
		case color:
			// if there was no previous color started and this one has no
			// arguments than just strip it because its invalid
			if firstarg == "" {
				return ""
			}

			b.Reset()
			// have to close previous color code since this is an "embedded" color code
			// a color code that was not reset before beginning a new one
			if len(state[controlcode]) > 0 {
				// we do not remove the closing tag since we would have to push it onto
				// the stack anyway
				b.WriteString(state[controlcode][0])
			} else {
				// push the closing tag onto the stack
				closetags := state[controlcode]
				closetags = append(closetags, "</span>")
				state[controlcode] = closetags
			}

			// write out the opening tag
			b.WriteString(`<span style="color: `)
			b.WriteString(colors[firstarg])
			if secondarg != "" {
				b.WriteString("; background-color: ")
				b.WriteString(colors[secondarg])
			}
			b.WriteString(`">`)
			return b.String()
		case reset:
			// write out all closing tags and pop them off the stack
			b.Reset()
			for k, a := range state {
				for _, v := range a {
					b.WriteString(v)
				}
				delete(state, k)
			}
			return b.String()
		}

		return m
	})

	// there were unclosed tags, so close them by pasting them to the end of
	// the string
	if len(state) > 0 {
		b.Reset()
		b.WriteString(s)
		for _, a := range state {
			for _, v := range a {
				b.WriteString(v)
			}
		}
		s = b.String()
	}

	return template.HTML(s)
}
