package main

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

func pyClass(n *sitter.Node, src []byte, file string) *ClassInfo {
	nameN := n.ChildByFieldName("name")
	if nameN == nil {
		return nil
	}
	return &ClassInfo{
		Name:    nameN.Content(src),
		File:    file,
		Line:    int(n.StartPoint().Row) + 1,
		Bases:   pyBases(n.ChildByFieldName("superclasses"), src),
		Methods: pyMethods(n.ChildByFieldName("body"), src),
	}
}

// pyBases lee la lista de superclases, descartando keyword args (metaclass=...)
// y desempaquetados (*args, **kwargs).
func pyBases(args *sitter.Node, src []byte) []baseRef {
	if args == nil {
		return nil
	}
	var out []baseRef
	for i := 0; i < int(args.NamedChildCount()); i++ {
		if b, ok := pyBase(args.NamedChild(i), src); ok {
			out = append(out, b)
		}
	}
	return out
}

func pyBase(n *sitter.Node, src []byte) (baseRef, bool) {
	if n == nil {
		return baseRef{}, false
	}
	switch n.Type() {
	case "keyword_argument", "list_splat", "dictionary_splat":
		return baseRef{}, false
	case "subscript": // Generic[T] -> Generic
		return pyBase(n.ChildByFieldName("value"), src)
	case "attribute": // threading.Thread -> Thread, calificador "threading"
		return baseRef{
			name: n.ChildByFieldName("attribute").Content(src),
			qual: n.ChildByFieldName("object").Content(src),
		}, true
	}
	return baseRef{name: n.Content(src)}, true
}

// pyMethods incluye métodos decorados (@property, @validator, @staticmethod...).
func pyMethods(body *sitter.Node, src []byte) []string {
	if body == nil {
		return nil
	}
	var out []string
	for i := 0; i < int(body.NamedChildCount()); i++ {
		m := body.NamedChild(i)
		if m.Type() == "decorated_definition" {
			m = m.ChildByFieldName("definition")
		}
		if m != nil && m.Type() == "function_definition" {
			if nm := m.ChildByFieldName("name"); nm != nil {
				out = append(out, nm.Content(src))
			}
		}
	}
	return out
}

// pyImports devuelve nombre local -> módulo para un import.
func pyImports(n *sitter.Node, src []byte) map[string]string {
	out := map[string]string{}
	switch n.Type() {
	case "import_statement": // import threading / import concurrent.futures as cf
		for i := 0; i < int(n.NamedChildCount()); i++ {
			c := n.NamedChild(i)
			switch c.Type() {
			case "dotted_name":
				mod := c.Content(src)
				out[strings.SplitN(mod, ".", 2)[0]] = mod
			case "aliased_import":
				out[c.ChildByFieldName("alias").Content(src)] = c.ChildByFieldName("name").Content(src)
			}
		}
	case "import_from_statement": // from pydantic import BaseModel
		mod := n.ChildByFieldName("module_name")
		if mod == nil {
			return out
		}
		modName := mod.Content(src)
		for i := 0; i < int(n.NamedChildCount()); i++ {
			c := n.NamedChild(i)
			if c.StartByte() == mod.StartByte() {
				continue
			}
			switch c.Type() {
			case "dotted_name":
				parts := strings.Split(c.Content(src), ".")
				out[parts[len(parts)-1]] = modName
			case "aliased_import":
				out[c.ChildByFieldName("alias").Content(src)] = modName
			}
		}
	}
	return out
}

// pyUsage clasifica un identificador según su nodo padre.
func pyUsage(n *sitter.Node, src []byte) string {
	p := n.Parent()
	if p == nil {
		return ""
	}
	switch p.Type() {
	case "call": // Perro()
		if sameStart(p.ChildByFieldName("function"), n) {
			return "instanciación"
		}
	case "attribute": // mod.Perro()
		if pp := p.Parent(); pp != nil && pp.Type() == "call" &&
			sameStart(pp.ChildByFieldName("function"), p) &&
			sameStart(p.ChildByFieldName("attribute"), n) {
			return "instanciación"
		}
	case "argument_list":
		pp := p.Parent()
		if pp == nil {
			return ""
		}
		switch pp.Type() {
		case "class_definition": // class Perro(Animal)
			return "herencia"
		case "call": // isinstance(x, Perro)
			if fn := pp.ChildByFieldName("function"); fn != nil {
				if s := fn.Content(src); s == "isinstance" || s == "issubclass" {
					return "chequeo de tipo"
				}
			}
		}
	case "type": // x: Perro
		return "anotación"
	}
	return ""
}

func sameStart(f, n *sitter.Node) bool {
	return f != nil && n != nil && f.StartByte() == n.StartByte()
}
