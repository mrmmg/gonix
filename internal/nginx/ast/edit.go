package ast

import "strings"

// Find returns the direct child directives called name.
func (l List) Find(name string) []*Node {
	var out []*Node
	for _, n := range l {
		if n.Is(name) {
			out = append(out, n)
		}
	}
	return out
}

// First returns the first direct child directive called name, or nil.
func (l List) First(name string) *Node {
	for _, n := range l {
		if n.Is(name) {
			return n
		}
	}
	return nil
}

// Walk visits every directive in depth-first order, descending into
// blocks. Returning false from fn skips that node's block.
func (l List) Walk(fn func(n *Node) bool) {
	for _, n := range l {
		if n.Kind != KindDirective {
			continue
		}
		if fn(n) && n.Block != nil {
			n.Block.Children.Walk(fn)
		}
	}
}

// Servers returns every `server` block in the file, wherever it sits
// (top level, inside `http`, ...).
func (f *File) Servers() []*Node {
	var out []*Node
	f.Children.Walk(func(n *Node) bool {
		if n.Name.Text == "server" && n.Block != nil {
			out = append(out, n)
			return false
		}
		return true
	})
	return out
}

// SetArgs replaces the arguments of n, keeping single-space separation and
// quoting each value only when it needs it. The directive's name, position
// and surrounding comments are untouched.
func (n *Node) SetArgs(values ...string) {
	args := make([]Token, len(values))
	for i, v := range values {
		args[i] = Token{Space: " ", Text: Quote(v)}
	}
	n.Args = args
}

// NewDirective builds `name arg...;`, indented with indent.
func NewDirective(indent, name string, values ...string) *Node {
	n := &Node{Kind: KindDirective, Name: Token{Space: indent, Text: name}}
	n.SetArgs(values...)
	return n
}

// Quote wraps v in double quotes when it is empty or has characters that
// would otherwise split or end the token.
func Quote(v string) string {
	if v != "" && !strings.ContainsAny(v, " \t\r\n;{}\"'#\\") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(v) + `"`
}

// Set gives the first `name` directive the arguments values, or, when
// there is none, appends `name values...;` indented like the siblings.
func (l *List) Set(name string, values ...string) *Node {
	if n := l.First(name); n != nil {
		n.SetArgs(values...)
		return n
	}
	n := NewDirective(l.indent(), name, values...)
	*l = append(*l, n)
	return n
}

// Remove deletes every direct child directive called name, together with
// a comment that follows it on the same line, and reports how many were
// removed.
func (l *List) Remove(name string) int {
	removed := 0
	for _, n := range l.Find(name) {
		if l.RemoveNode(n) {
			removed++
		}
	}
	return removed
}

// RemoveNode deletes n from the list, together with a comment that follows
// it on the same line (e.g. "# managed by Certbot"), so the comment is not
// left behind describing whatever line comes before it. It reports whether
// n was found.
func (l *List) RemoveNode(n *Node) bool {
	i := l.Index(n)
	if i < 0 {
		return false
	}
	end := i + 1
	if end < len(*l) && (*l)[end].IsTrailingComment() {
		end++
	}
	*l = append((*l)[:i:i], (*l)[end:]...)
	return true
}

// Index returns the position of n in the list, or -1.
func (l List) Index(n *Node) int {
	for i, c := range l {
		if c == n {
			return i
		}
	}
	return -1
}

// IsTrailingComment reports whether n is a comment on the same line as the
// node before it.
func (n *Node) IsTrailingComment() bool {
	return n.Kind == KindComment && !strings.Contains(n.Name.Space, "\n")
}

// TrailingComment returns the same-line comment that follows n in l, or nil.
func (l List) TrailingComment(n *Node) *Node {
	i := l.Index(n)
	if i < 0 || i+1 >= len(l) || !l[i+1].IsTrailingComment() {
		return nil
	}
	return l[i+1]
}

// InsertAt puts nodes at position i (clamped to the list bounds).
func (l *List) InsertAt(i int, nodes ...*Node) {
	i = max(0, min(i, len(*l)))
	out := make(List, 0, len(*l)+len(nodes))
	out = append(out, (*l)[:i]...)
	out = append(out, nodes...)
	*l = append(out, (*l)[i:]...)
}

// Replace swaps old for the given nodes in place; it reports whether old
// was found. A same-line comment after old is kept.
func (l *List) Replace(old *Node, nodes ...*Node) bool {
	i := l.Index(old)
	if i < 0 {
		return false
	}
	rest := append(List{}, (*l)[i+1:]...)
	*l = append(append((*l)[:i:i], nodes...), rest...)
	return true
}

// After returns the index just past n and its same-line comment, i.e.
// where a node meant to follow n should go; -1 when n is not in l.
func (l List) After(n *Node) int {
	i := l.Index(n)
	if i < 0 {
		return -1
	}
	if i+1 < len(l) && l[i+1].IsTrailingComment() {
		return i + 2
	}
	return i + 1
}

// ParseNodes parses a configuration fragment, e.g. the body of a block,
// returning its nodes and trailing whitespace.
func ParseNodes(src string) (List, string, error) {
	f, err := Parse(src)
	if err != nil {
		return nil, "", err
	}
	return f.Children, f.Tail, nil
}

// Indent returns the indentation (without the newline) of the last node
// in l that starts on its own line, or fallback when there is none.
func (l List) Indent(fallback string) string {
	for i := len(l) - 1; i >= 0; i-- {
		sp := l[i].Name.Space
		if j := strings.LastIndexByte(sp, '\n'); j >= 0 {
			return sp[j+1:]
		}
	}
	return fallback
}

// indent is the whitespace, newline included, that lines a new node up
// with its siblings.
func (l List) indent() string { return "\n" + l.Indent("    ") }
