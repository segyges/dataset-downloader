package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	termbox "github.com/nsf/termbox-go"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/taylorskalyo/goreader/epub"
)

// parser is a part of the goreader repo for parsing epubs
type Parser struct {
	tagStack  []atom.Atom
	tokenizer *html.Tokenizer
	doc       Cellbuf
	items     []epub.Item
	sb        strings.Builder
}

// cellbuf is a part of the goreader repo for parsing epubs
type Cellbuf struct {
	cells   []termbox.Cell
	width   int
	lmargin int
	col     int
	row     int
	space   bool
	fg, bg  termbox.Attribute
}

// parseText takes in html content via an io.Reader and returns a buffer
// containing only plain text.
func ParseText(r io.Reader, items []epub.Item, sb strings.Builder) (strings.Builder, error) {
	tokenizer := html.NewTokenizer(r)
	doc := Cellbuf{width: 80}
	p := Parser{tokenizer: tokenizer, doc: doc, items: items, sb: sb}
	err := p.Parse()
	if err != nil {
		return p.sb, err
	}
	return p.sb, nil
}

// parse walks an html document and renders elements to a cell buffer document.
func (p *Parser) Parse() (err error) {
	for {
		tokenType := p.tokenizer.Next()
		token := p.tokenizer.Token()
		switch tokenType {
		case html.ErrorToken:
			err = p.tokenizer.Err()
		case html.StartTagToken:
			p.tagStack = append(p.tagStack, token.DataAtom) // push element
			fallthrough
		case html.SelfClosingTagToken:
			p.HandleStartTag(token)
		case html.TextToken:
			p.HandleText(token)
		case html.EndTagToken:
			p.tagStack = p.tagStack[:len(p.tagStack)-1] // pop element
		}
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
	}
}

// handleText appends text elements to the parser buffer. It filters elements
// that should not be displayed as text (e.g. style blocks).
func (p *Parser) HandleText(token html.Token) {
	// Skip style tags
	if len(p.tagStack) > 0 && p.tagStack[len(p.tagStack)-1] == atom.Style {
		return
	}
	p.doc.Style(p.tagStack)
	// I think the appendText is needed to properly parse the tags
	p.doc.AppendText(string(token.Data))
	p.sb.WriteString(string(token.Data))

}

// handleStartTag appends text representations of non-text elements (e.g. image alt
// tags) to the parser buffer.
func (p *Parser) HandleStartTag(token html.Token) {
	switch token.DataAtom {
	case atom.Img:
		// Display alt text in place of images.
		for _, a := range token.Attr {
			switch atom.Lookup([]byte(a.Key)) {
			case atom.Alt:
				text := fmt.Sprintf("Alt text: %s", a.Val)
				p.doc.AppendText(text)
				p.doc.row++
				p.doc.col = p.doc.lmargin
			case atom.Src:
				for _, item := range p.items {
					if item.HREF == a.Val {

						break
					}
				}
			}
		}
	case atom.Br:
		p.doc.row++
		p.doc.col = p.doc.lmargin
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Title,
		atom.Div, atom.Tr:
		p.doc.row += 2
		p.doc.col = p.doc.lmargin
	case atom.P:
		p.doc.row += 2
		p.doc.col = p.doc.lmargin
		p.doc.col += 2
	case atom.Hr:
		p.doc.row++
		p.doc.col = 0
		p.doc.AppendText(strings.Repeat("-", p.doc.width))
	}
}

// style sets the foreground/background attributes for future cells in the cell
// buffer document based on HTML tags in the tag stack.
func (c *Cellbuf) Style(tags []atom.Atom) {
	fg := termbox.ColorDefault
	for _, tag := range tags {
		switch tag {
		case atom.B, atom.Strong, atom.Em:
			fg |= termbox.AttrBold
		case atom.I:
			fg |= termbox.ColorYellow
		case atom.Title:
			fg |= termbox.ColorRed
		case atom.H1:
			fg |= termbox.ColorMagenta
		case atom.H2:
			fg |= termbox.ColorBlue
		case atom.H3, atom.H4, atom.H5, atom.H6:
			fg |= termbox.ColorCyan
		}
	}
	c.fg = fg
}

// appendText appends text to the cell buffer document.
func (c *Cellbuf) AppendText(str string) {
	if len(str) <= 0 {
		return
	}
	if c.col < c.lmargin {
		c.col = c.lmargin
	}

	scanner := bufio.NewScanner(strings.NewReader(str))
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() {
		if c.col != c.lmargin && c.space {
			c.col++
		}
		word := []rune(scanner.Text())
		if len(word) > c.width-c.col {
			c.row++
			c.col = c.lmargin
		}
		for _, r := range word {
			c.SetCell(c.col, c.row, r, c.fg, c.bg)
			c.col++
		}
	}
}

// setCell changes a cell's attributes in the cell buffer document at the given
// position.
func (c *Cellbuf) SetCell(x, y int, ch rune, fg, bg termbox.Attribute) {
	// Grow in steps of 1024 when out of space.
	for y*c.width+x >= len(c.cells) {
		c.cells = append(c.cells, make([]termbox.Cell, 1024)...)
	}
	c.cells[y*c.width+x] = termbox.Cell{Ch: ch, Fg: fg, Bg: bg}
}
