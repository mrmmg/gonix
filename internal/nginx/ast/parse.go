package ast

import (
	"fmt"
	"strings"
)

// ParseError reports where a file stopped making sense.
type ParseError struct {
	Line int
	Msg  string
}

func (e *ParseError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

type parser struct {
	src string
	pos int
}

func (p *parser) line() int {
	end := min(p.pos, len(p.src))
	return 1 + strings.Count(p.src[:end], "\n")
}

func (p *parser) fail(msg string) error { return &ParseError{Line: p.line(), Msg: msg} }

// Parse reads a configuration file. It never panics; input it cannot
// represent faithfully yields an error, and the caller should then treat
// the file as opaque.
func Parse(src string) (*File, error) {
	p := &parser{src: src}
	children, tail, err := p.parseList(0)
	if err != nil {
		return nil, err
	}
	return &File{Children: children, Tail: tail}, nil
}

func (p *parser) space() string {
	start := p.pos
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case ' ', '\t', '\r', '\n':
			p.pos++
		default:
			return p.src[start:p.pos]
		}
	}
	return p.src[start:p.pos]
}

// parseList reads nodes until EOF (depth 0) or an unconsumed '}' (depth>0).
// The returned string is the whitespace before that end.
func (p *parser) parseList(depth int) (List, string, error) {
	var out List
	for {
		sp := p.space()
		if p.pos >= len(p.src) {
			if depth > 0 {
				return nil, "", p.fail("unexpected end of file, expecting \"}\"")
			}
			return out, sp, nil
		}
		switch c := p.src[p.pos]; c {
		case '}':
			if depth == 0 {
				return nil, "", p.fail("unexpected \"}\"")
			}
			return out, sp, nil
		case ';', '{':
			return nil, "", p.fail(fmt.Sprintf("unexpected %q", string(c)))
		case '#':
			start := p.pos
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
			out = append(out, &Node{Kind: KindComment, Name: Token{Space: sp, Text: p.src[start:p.pos]}})
			continue
		}
		n, err := p.parseDirective(sp, depth)
		if err != nil {
			return nil, "", err
		}
		out = append(out, n)
	}
}

func (p *parser) parseDirective(sp string, depth int) (*Node, error) {
	name, err := p.word()
	if err != nil {
		return nil, err
	}
	n := &Node{Kind: KindDirective, Name: Token{Space: sp, Text: name}}
	for {
		argSp := p.space()
		if p.pos >= len(p.src) {
			return nil, p.fail(fmt.Sprintf("unexpected end of file, expecting \";\" or \"{\" after %q", name))
		}
		switch p.src[p.pos] {
		case ';':
			p.pos++
			n.SemiSpace = argSp
			return n, nil
		case '{':
			p.pos++
			b := &Block{OpenSpace: argSp}
			if strings.HasSuffix(name, "_block") {
				body, err := p.rawBody()
				if err != nil {
					return nil, err
				}
				b.RawBody, b.HasRawBody = body, true
			} else {
				children, closeSp, err := p.parseList(depth + 1)
				if err != nil {
					return nil, err
				}
				b.Children, b.CloseSpace = children, closeSp
				p.pos++ // '}'
			}
			n.Block = b
			return n, nil
		case '}':
			return nil, p.fail(fmt.Sprintf("unexpected \"}\" after %q (missing \";\")", name))
		case '#':
			return nil, p.fail("comment inside a directive is not supported")
		}
		w, err := p.word()
		if err != nil {
			return nil, err
		}
		n.Args = append(n.Args, Token{Space: argSp, Text: w})
	}
}

// word reads one token. A leading quote reads to the matching quote; the
// rest of the word (e.g. the ")" in `"https")`) stays attached, as in Nginx.
func (p *parser) word() (string, error) {
	start := p.pos
	if q := p.src[p.pos]; q == '"' || q == '\'' {
		p.pos++
		for {
			if p.pos >= len(p.src) {
				return "", p.fail("unterminated quoted string")
			}
			c := p.src[p.pos]
			p.pos++
			if c == '\\' && p.pos < len(p.src) {
				p.pos++
				continue
			}
			if c == q {
				break
			}
		}
	}
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch c {
		case ' ', '\t', '\r', '\n', ';', '{':
			return p.src[start:p.pos], nil
		case '\\':
			p.pos += 2
			if p.pos > len(p.src) {
				p.pos = len(p.src)
			}
		case '$':
			p.pos++
			if p.pos < len(p.src) && p.src[p.pos] == '{' {
				end := strings.IndexByte(p.src[p.pos:], '}')
				if end < 0 {
					return "", p.fail("unterminated \"${\"")
				}
				p.pos += end + 1
			}
		default:
			p.pos++
		}
	}
	return p.src[start:p.pos], nil
}

// rawBody reads a Lua-style block body up to its matching '}', which it
// consumes. Braces inside strings and `--` comments are ignored.
func (p *parser) rawBody() (string, error) {
	start, depth := p.pos, 1
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case c == '"' || c == '\'':
			p.pos++
			for p.pos < len(p.src) && p.src[p.pos] != c && p.src[p.pos] != '\n' {
				if p.src[p.pos] == '\\' {
					p.pos++
				}
				p.pos++
			}
		case c == '-' && strings.HasPrefix(p.src[p.pos:], "--"):
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
			continue
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				body := p.src[start:p.pos]
				p.pos++
				return body, nil
			}
		}
		p.pos++
	}
	return "", p.fail("unterminated block body")
}
