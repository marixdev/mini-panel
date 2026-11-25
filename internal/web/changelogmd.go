package web

import (
	"html"
	"html/template"
	"regexp"
	"strings"
)

// changelogHTML turns the part of the changelog about one release into HTML. It knows only what CHANGELOG.md uses: "### " headings,
// "- " items (an indented line goes on with the item before), paragraphs, **bold**, `code` and [text](https://...) links. Everything
// is escaped first, and a link is kept only when it goes to https, so a changelog cannot put markup or a script into the page.
func changelogHTML(md string) template.HTML {
	var b strings.Builder
	inList := false
	var item []string
	var para []string
	flushItem := func() {
		if len(item) > 0 {
			b.WriteString("<li>" + inlineMD(strings.Join(item, " ")) + "</li>")
			item = nil
		}
	}
	closeList := func() {
		flushItem()
		if inList {
			b.WriteString("</ul>")
			inList = false
		}
	}
	flushPara := func() {
		if len(para) > 0 {
			b.WriteString("<p>" + inlineMD(strings.Join(para, " ")) + "</p>")
			para = nil
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "":
			closeList()
			flushPara()
		case strings.HasPrefix(t, "#"):
			closeList()
			flushPara()
			b.WriteString("<h4>" + inlineMD(strings.TrimSpace(strings.TrimLeft(t, "#"))) + "</h4>")
		case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* "):
			flushPara()
			flushItem()
			if !inList {
				b.WriteString("<ul>")
				inList = true
			}
			item = []string{t[2:]}
		case inList && len(item) > 0 && line != t: // an indented line goes on with the item
			item = append(item, t)
		default:
			closeList()
			para = append(para, t)
		}
	}
	closeList()
	flushPara()
	return template.HTML(b.String()) // #nosec G203 -- built from escaped text only
}

var (
	mdCode = regexp.MustCompile("`([^`]+)`")
	mdBold = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	mdLink = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
)

func inlineMD(s string) string {
	s = html.EscapeString(s)
	// Code first, so what is inside it is not read as bold or a link. Its text is kept aside and put back at the end.
	var codes []string
	s = mdCode.ReplaceAllStringFunc(s, func(m string) string {
		codes = append(codes, "<code>"+mdCode.FindStringSubmatch(m)[1]+"</code>")
		return "\x00" + string(rune('0'+len(codes)-1)) + "\x00"
	})
	s = mdLink.ReplaceAllStringFunc(s, func(m string) string {
		p := mdLink.FindStringSubmatch(m)
		if !strings.HasPrefix(p[2], "https://") {
			return p[1]
		}
		return `<a class="link" href="` + p[2] + `" target="_blank" rel="noopener noreferrer">` + p[1] + `</a>`
	})
	s = mdBold.ReplaceAllString(s, "<strong>$1</strong>")
	for i, c := range codes {
		s = strings.Replace(s, "\x00"+string(rune('0'+i))+"\x00", c, 1)
	}
	return s
}
