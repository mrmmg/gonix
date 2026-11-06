package ast

import "strings"

// Print renders the file exactly as it was parsed (plus any edits).
func Print(f *File) string {
	var b strings.Builder
	f.Children.write(&b)
	b.WriteString(f.Tail)
	return b.String()
}

// String renders a single node, whitespace before it included.
func (n *Node) String() string {
	var b strings.Builder
	n.write(&b)
	return b.String()
}

func (l List) write(b *strings.Builder) {
	for _, n := range l {
		n.write(b)
	}
}

func (n *Node) write(b *strings.Builder) {
	b.WriteString(n.Name.Space)
	b.WriteString(n.Name.Text)
	if n.Kind == KindComment {
		return
	}
	for _, a := range n.Args {
		b.WriteString(a.Space)
		b.WriteString(a.Text)
	}
	if n.Block == nil {
		b.WriteString(n.SemiSpace)
		b.WriteByte(';')
		return
	}
	b.WriteString(n.Block.OpenSpace)
	b.WriteByte('{')
	if n.Block.HasRawBody {
		b.WriteString(n.Block.RawBody)
	} else {
		n.Block.Children.write(b)
		b.WriteString(n.Block.CloseSpace)
	}
	b.WriteByte('}')
}
