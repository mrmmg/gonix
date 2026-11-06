// Package ast is a lossless parser for Nginx configuration files.
//
// Every byte of the input — whitespace, comments, quoting — is kept, so
// Print(Parse(x)) == x for any input Parse accepts. Editing helpers change
// only the node they target, which lets GoNix manage hand-written files
// without reformatting or dropping anything it does not understand.
package ast

import "strings"

// Kind tells a directive from a comment.
type Kind int

const (
	KindDirective Kind = iota
	KindComment
)

// Token is one word of a directive, with the whitespace before it.
type Token struct {
	Space string
	Text  string // exactly as written, quotes included
}

// Value is the token's text without surrounding quotes and escapes.
func (t Token) Value() string { return unquote(t.Text) }

// Node is a directive (`name args;` or `name args { ... }`) or a comment.
// For a comment, Name.Text is the whole comment including the leading '#'.
type Node struct {
	Kind      Kind
	Name      Token
	Args      []Token
	SemiSpace string // whitespace before ';' (directives without a block)
	Block     *Block // nil for a plain `name args;` directive
}

// Block is the `{ ... }` body of a directive.
type Block struct {
	OpenSpace  string // whitespace before '{'
	Children   List
	CloseSpace string // whitespace before '}'
	RawBody    string // verbatim body of a *_block directive (Lua); Children is empty
	HasRawBody bool
}

// File is a parsed configuration file.
type File struct {
	Children List
	Tail     string // trailing whitespace after the last node
}

// List is an ordered sequence of nodes.
type List []*Node

// Is reports whether n is a directive called name.
func (n *Node) Is(name string) bool { return n.Kind == KindDirective && n.Name.Text == name }

// Values returns the unquoted arguments.
func (n *Node) Values() []string {
	out := make([]string, len(n.Args))
	for i, a := range n.Args {
		out[i] = a.Value()
	}
	return out
}

// Arg returns the i-th unquoted argument, or "" when missing.
func (n *Node) Arg(i int) string {
	if i < 0 || i >= len(n.Args) {
		return ""
	}
	return n.Args[i].Value()
}

func unquote(s string) string {
	if s == "" {
		return s
	}
	q := s[0]
	if q != '"' && q != '\'' {
		return s
	}
	var b strings.Builder
	i := 1
	for ; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		if c == q {
			i++
			break
		}
		b.WriteByte(c)
	}
	b.WriteString(s[i:])
	return b.String()
}
