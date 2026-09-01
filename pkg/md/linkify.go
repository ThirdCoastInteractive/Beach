package md

import (
	"bytes"
	"regexp"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// GFM autolink patterns (protocol URLs and www.).
var (
	wwwURLRegexp = regexp.MustCompile(`^www\.[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-z]+(?:[/#?][-a-zA-Z0-9@:%_\+.~#!?&/=\(\);,'">\^{}\[\]` + "`" + `]*)?`)
	urlRegexp    = regexp.MustCompile(`^https?://[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-z]+(?::\d+)?(?:[/#?][-a-zA-Z0-9@:%_+.~#$!?&/=\(\);,'">\^{}\[\]` + "`" + `]*)?`)
)

type linkifyParser struct{}

func (linkifyParser) Trigger() []byte {
	return []byte{' ', '*', '_', '~', '('}
}

func (linkifyParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	if pc.IsInLinkLabel() {
		return nil
	}
	line, segment := block.PeekLine()
	if len(line) == 0 {
		return nil
	}
	consumes := 0
	start := segment.Start
	c := line[0]
	if c == ' ' || c == '*' || c == '_' || c == '~' || c == '(' {
		consumes++
		start++
		line = line[1:]
	}
	var m []int
	var protocol []byte
	if bytes.HasPrefix(line, []byte("http:")) || bytes.HasPrefix(line, []byte("https:")) {
		m = urlRegexp.FindSubmatchIndex(line)
	}
	if m == nil && bytes.HasPrefix(line, []byte("www.")) {
		m = wwwURLRegexp.FindSubmatchIndex(line)
		protocol = []byte("http")
	}
	if m == nil || m[0] != 0 {
		return nil
	}
	lastChar := line[m[1]-1]
	switch lastChar {
	case '.':
		m[1]--
	case ')':
		closing := 0
		for i := m[1] - 1; i >= m[0]; i-- {
			switch line[i] {
			case ')':
				closing++
			case '(':
				closing--
			}
		}
		if closing > 0 {
			m[1] -= closing
		}
	case ';':
		i := m[1] - 2
		for ; i >= m[0]; i-- {
			if util.IsAlphaNumeric(line[i]) {
				continue
			}
			break
		}
		if i != m[1]-2 && line[i] == '&' {
			m[1] = i
		}
	}
	if consumes != 0 {
		s := segment.WithStop(segment.Start + 1)
		ast.MergeOrAppendTextSegment(parent, s)
	}
	i := m[1] - 1
	for ; i > 0; i-- {
		switch line[i] {
		case '?', '!', '.', ',', ':', '*', '_', '~':
		default:
			goto endfor
		}
	}
endfor:
	i++
	consumes += i
	block.Advance(consumes)
	n := ast.NewTextSegment(text.NewSegment(start, start+i))
	link := ast.NewAutoLink(ast.AutoLinkURL, n)
	link.Protocol = protocol
	return link
}
