package md

import (
	"bytes"
	"regexp"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// gfm adds GFM tables and autolinks on top of goldmark's CommonMark core.
type gfm struct{}

func (gfm) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithParagraphTransformers(
			util.Prioritized(tableParagraphTransformer{}, 200),
		),
		parser.WithInlineParsers(
			util.Prioritized(linkifyParser{}, 999),
		),
	)
	m.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(tableHTMLRenderer{}, 500),
	))
}

var (
	kindTable       = ast.NewNodeKind("Table")
	kindTableHeader = ast.NewNodeKind("TableHeader")
	kindTableRow    = ast.NewNodeKind("TableRow")
	kindTableCell   = ast.NewNodeKind("TableCell")

	tableDelimLeft   = regexp.MustCompile(`^\s*:--*\s*$`)
	tableDelimRight  = regexp.MustCompile(`^\s*--*:\s*$`)
	tableDelimCenter = regexp.MustCompile(`^\s*:--*:\s*$`)
	tableDelimNone   = regexp.MustCompile(`^\s*-+\s*$`)
)

type tableNode struct{ ast.BaseBlock }

func (n *tableNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}
func (n *tableNode) Kind() ast.NodeKind { return kindTable }

type tableHeader struct{ ast.BaseBlock }

func (n *tableHeader) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}
func (n *tableHeader) Kind() ast.NodeKind { return kindTableHeader }

type tableRow struct{ ast.BaseBlock }

func (n *tableRow) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}
func (n *tableRow) Kind() ast.NodeKind { return kindTableRow }

type tableCell struct{ ast.BaseBlock }

func (n *tableCell) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}
func (n *tableCell) Kind() ast.NodeKind { return kindTableCell }

type tableParagraphTransformer struct{}

func (tableParagraphTransformer) Transform(node *ast.Paragraph, reader text.Reader, _ parser.Context) {
	lines := node.Lines()
	if lines.Len() < 2 {
		return
	}
	for i := 1; i < lines.Len(); i++ {
		alignments := parseDelimiter(lines.At(i), reader)
		if alignments == nil {
			continue
		}
		header := parseRow(lines.At(i-1), len(alignments), true, reader)
		if header == nil || header.ChildCount() != len(alignments) {
			return
		}
		table := &tableNode{}
		th := &tableHeader{}
		for c := header.FirstChild(); c != nil; {
			next := c.NextSibling()
			th.AppendChild(th, c)
			c = next
		}
		table.AppendChild(table, th)
		for j := i + 1; j < lines.Len(); j++ {
			table.AppendChild(table, parseRow(lines.At(j), len(alignments), false, reader))
		}
		node.Lines().SetSliced(0, i-1)
		node.Parent().InsertAfter(node.Parent(), node, table)
		if node.Lines().Len() == 0 {
			node.Parent().RemoveChild(node.Parent(), node)
		} else {
			last := node.Lines().At(i - 2)
			last.Stop--
			node.Lines().Set(i-2, last)
		}
		return
	}
}

func isTableDelim(bs []byte) bool {
	if w, _ := util.IndentWidth(bs, 0); w > 3 {
		return false
	}
	allSep := true
	for _, b := range bs {
		if b != '-' {
			allSep = false
		}
		if !(util.IsSpace(b) || b == '-' || b == '|' || b == ':') {
			return false
		}
	}
	return !allSep
}

func parseDelimiter(segment text.Segment, reader text.Reader) []bool {
	line := segment.Value(reader.Source())
	if !isTableDelim(line) {
		return nil
	}
	cols := bytes.Split(line, []byte{'|'})
	if util.IsBlank(cols[0]) {
		cols = cols[1:]
	}
	if len(cols) > 0 && util.IsBlank(cols[len(cols)-1]) {
		cols = cols[:len(cols)-1]
	}
	alignments := make([]bool, 0, len(cols))
	for _, col := range cols {
		if tableDelimLeft.Match(col) || tableDelimRight.Match(col) ||
			tableDelimCenter.Match(col) || tableDelimNone.Match(col) {
			alignments = append(alignments, true)
			continue
		}
		return nil
	}
	return alignments
}

func parseRow(segment text.Segment, cols int, isHeader bool, reader text.Reader) *tableRow {
	source := reader.Source()
	segment = segment.TrimLeftSpace(source)
	segment = segment.TrimRightSpace(source)
	line := segment.Value(source)
	pos := 0
	limit := len(line)
	row := &tableRow{}
	if len(line) > 0 && line[pos] == '|' {
		pos++
	}
	if len(line) > 0 && line[limit-1] == '|' {
		limit--
	}
	i := 0
	for ; pos < limit; i++ {
		if i >= cols && !isHeader {
			return row
		}
		closure := pos
		for ; closure < limit; closure++ {
			if line[closure] == '|' && (closure == 0 || line[closure-1] != '\\') {
				break
			}
		}
		cell := &tableCell{}
		seg := text.NewSegment(segment.Start+pos, segment.Start+closure)
		seg = seg.TrimLeftSpace(source)
		seg = seg.TrimRightSpace(source)
		cell.Lines().Append(seg)
		row.AppendChild(row, cell)
		pos = closure + 1
	}
	for ; i < cols; i++ {
		row.AppendChild(row, &tableCell{})
	}
	return row
}

type tableHTMLRenderer struct{}

func (tableHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindTable, renderTable)
	reg.Register(kindTableHeader, renderTableHeader)
	reg.Register(kindTableRow, renderTableRow)
	reg.Register(kindTableCell, renderTableCell)
}

func renderTable(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<table>\n")
	} else {
		_, _ = w.WriteString("</table>\n")
	}
	return ast.WalkContinue, nil
}

func renderTableHeader(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<thead>\n<tr>\n")
	} else {
		_, _ = w.WriteString("</tr>\n</thead>\n")
		if n.NextSibling() != nil {
			_, _ = w.WriteString("<tbody>\n")
		}
	}
	return ast.WalkContinue, nil
}

func renderTableRow(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<tr>\n")
	} else {
		_, _ = w.WriteString("</tr>\n")
		if n.Parent().LastChild() == n {
			_, _ = w.WriteString("</tbody>\n")
		}
	}
	return ast.WalkContinue, nil
}

func renderTableCell(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	tag := "td"
	if n.Parent().Kind() == kindTableHeader {
		tag = "th"
	}
	if entering {
		_, _ = w.WriteString("<" + tag + ">")
	} else {
		_, _ = w.WriteString("</" + tag + ">\n")
	}
	return ast.WalkContinue, nil
}
