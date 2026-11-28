package main

import (
	sitter "github.com/smacker/go-tree-sitter"
	"strings"
)

// Sirve para JavaScript, TypeScript y TSX.
func jsClass(n *sitter.Node, src []byte, file string) *ClassInfo {
	nameN := n.ChildByFieldName("name")
	if nameN == nil {
		return nil
	}
	return &ClassInfo{
		Name:    nameN.Content(src),
		File:    file,
		Line:    int(n.StartPoint().Row) + 1,
		Bases:   jsBases(n, src),
		Methods: jsMethods(n.ChildByFieldName("body"), src),
	}
}

// jsBases lee class_heritage. En TS, implements no es herencia de clase.
func jsBases(class *sitter.Node, src []byte) []baseRef {
	var out []baseRef
	for i := 0; i < int(class.NamedChildCount()); i++ {
		h := class.NamedChild(i)
		if h.Type() != "class_heritage" {
			continue
		}
		for j := 0; j < int(h.NamedChildCount()); j++ {
			c := h.NamedChild(j)
			switch c.Type() {
			case "implements_clause":
				// interfaces, no clases base
			case "extends_clause": // TS
				v := c.ChildByFieldName("value")
				if v == nil && c.NamedChildCount() > 0 {
					v = c.NamedChild(0)
				}
				if v != nil {
					out = append(out, jsBase(v, src))
				}
			default: // JS: la expresión va directa
				out = append(out, jsBase(c, src))
			}
		}
	}
	return out
}

func jsBase(n *sitter.Node, src []byte) baseRef {
	if n.Type() == "member_expression" { // mod.Base
		return baseRef{
			name: n.ChildByFieldName("property").Content(src),
			qual: n.ChildByFieldName("object").Content(src),
		}
	}
	return baseRef{name: n.Content(src)}
}

func jsMethods(body *sitter.Node, src []byte) []string {
	if body == nil {
		return nil
	}
	var out []string
	for i := 0; i < int(body.NamedChildCount()); i++ {
		m := body.NamedChild(i)
		if m.Type() != "method_definition" && m.Type() != "method_signature" {
			continue
		}
		if nm := m.ChildByFieldName("name"); nm != nil {
			out = append(out, nm.Content(src))
		}
	}
	return out
}

// jsImports: default, namespace (* as x) y named (import { a as b }).
// No cubre require() de CommonJS.
func jsImports(n *sitter.Node, src []byte) map[string]string {
	out := map[string]string{}
	if n.Type() != "import_statement" {
		return out
	}
	srcNode := n.ChildByFieldName("source")
	if srcNode == nil {
		return out
	}
	mod := strings.Trim(srcNode.Content(src), "\"'`")
	for i := 0; i < int(n.NamedChildCount()); i++ {
		clause := n.NamedChild(i)
		if clause.Type() != "import_clause" {
			continue
		}
		for j := 0; j < int(clause.NamedChildCount()); j++ {
			c := clause.NamedChild(j)
			switch c.Type() {
			case "identifier":
				out[c.Content(src)] = mod
			case "namespace_import":
				for k := 0; k < int(c.NamedChildCount()); k++ {
					out[c.NamedChild(k).Content(src)] = mod
				}
			case "named_imports":
				for k := 0; k < int(c.NamedChildCount()); k++ {
					spec := c.NamedChild(k)
					local := spec.ChildByFieldName("alias")
					if local == nil {
						local = spec.ChildByFieldName("name")
					}
					if local != nil {
						out[local.Content(src)] = mod
					}
				}
			}
		}
	}
	return out
}

// jsUsage clasifica un identificador según su nodo padre.
func jsUsage(n *sitter.Node, src []byte) string {
	p := n.Parent()
	if p == nil {
		return ""
	}
	switch p.Type() {
	case "new_expression": // new Perro()
		if sameStart(p.ChildByFieldName("constructor"), n) {
			return "instanciación"
		}
	case "class_heritage", "extends_clause": // class Perro extends Animal
		return "herencia"
	case "type_annotation", "generic_type": // x: Perro (TS), Lista<Perro>
		return "anotación"
	case "binary_expression": // x instanceof Perro
		if op := p.ChildByFieldName("operator"); op != nil && op.Type() == "instanceof" &&
			sameStart(p.ChildByFieldName("right"), n) {
			return "chequeo de tipo"
		}
	}
	return ""
}
