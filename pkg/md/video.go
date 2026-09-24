package md

import (
	"bytes"
	"html"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const videoPrefix = ":::video"

var kindVideo = ast.NewNodeKind("Video")

const iframeAllow = "accelerometer; gyroscope; autoplay; encrypted-media; picture-in-picture;"

// videoNode is a single-line `:::video <uid>` block.
type videoNode struct {
	ast.BaseBlock
	UID string
}

func (n *videoNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"UID": n.UID}, nil)
}

func (n *videoNode) Kind() ast.NodeKind { return kindVideo }

type videoParser struct{}

func (videoParser) Trigger() []byte { return []byte{':'} }

func (videoParser) Open(_ ast.Node, reader text.Reader, _ parser.Context) (ast.Node, parser.State) {
	line, _ := reader.PeekLine()
	w, pos := util.IndentWidth(line, reader.LineOffset())
	if w > 3 {
		return nil, parser.NoChildren
	}
	rest := bytes.TrimRight(line[pos:], "\r\n")
	if !bytes.HasPrefix(rest, []byte(videoPrefix)) {
		return nil, parser.NoChildren
	}
	rest = rest[len(videoPrefix):]
	if len(rest) == 0 || (rest[0] != ' ' && rest[0] != '\t') {
		return nil, parser.NoChildren
	}
	uid := bytes.TrimSpace(rest)
	if !tokenRE.Match(uid) {
		return nil, parser.NoChildren
	}
	reader.AdvanceToEOL()
	return &videoNode{UID: string(uid)}, parser.NoChildren
}

func (videoParser) Continue(ast.Node, text.Reader, parser.Context) parser.State {
	return parser.Close
}

func (videoParser) Close(ast.Node, text.Reader, parser.Context) {}

func (videoParser) CanInterruptParagraph() bool { return true }

func (videoParser) CanAcceptIndentedLine() bool { return false }

type videoExt struct {
	customer string
}

func (e videoExt) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithBlockParsers(
		util.Prioritized(videoParser{}, 150),
	))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(videoRenderer{customer: e.customer}, 500),
	))
}

type videoRenderer struct {
	customer string
}

func (r videoRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindVideo, r.render)
}

func (r videoRenderer) render(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	if !tokenRE.MatchString(r.customer) {
		return ast.WalkContinue, nil
	}
	uid := n.(*videoNode).UID
	src := "https://customer-" + r.customer + ".cloudflarestream.com/" + uid + "/iframe"
	_, _ = w.WriteString(`<iframe src="`)
	_, _ = w.WriteString(html.EscapeString(src))
	_, _ = w.WriteString(`" allow="`)
	_, _ = w.WriteString(iframeAllow)
	_, _ = w.WriteString(`" allowfullscreen loading="lazy"></iframe>` + "\n")
	return ast.WalkContinue, nil
}
