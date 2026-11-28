package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	sitter "github.com/smacker/go-tree-sitter"
)

// baseRef es una clase base tal como aparece en el código.
type baseRef struct {
	name string // nombre sin calificar: "Base" en mod.Base
	qual string // calificador: "mod" en mod.Base, o "" si no hay
}

type ClassInfo struct {
	Name, File, Lang string
	Line             int
	Bases            []baseRef
	Methods          []string
	imports          map[string]string // imports del archivo: nombre local -> módulo
}

type Index struct {
	Classes  []*ClassInfo
	Defs     map[string][]*ClassInfo   // nombre -> definiciones (puede haber varias)
	Mentions map[string]int            // identificadores con ese nombre, incluidas las definiciones
	Usos     map[string]map[string]int // nombre -> tipo de uso -> conteo
	Children map[string][]*ClassInfo   // nombre de base -> subclases directas
	Archivos int
	Errores  []string
}

func newIndex() *Index {
	return &Index{
		Defs:     map[string][]*ClassInfo{},
		Mentions: map[string]int{},
		Usos:     map[string]map[string]int{},
		Children: map[string][]*ClassInfo{},
	}
}

func (idx *Index) addClass(c *ClassInfo) {
	idx.Classes = append(idx.Classes, c)
	idx.Defs[c.Name] = append(idx.Defs[c.Name], c)
	for _, b := range c.Bases {
		idx.Children[b.name] = append(idx.Children[b.name], c)
	}
}

func (idx *Index) addUso(name, kind string) {
	if idx.Usos[name] == nil {
		idx.Usos[name] = map[string]int{}
	}
	idx.Usos[name][kind]++
}

// allSubclasses devuelve todas las subclases, directas e indirectas.
// A <- B <- C devuelve [B, C]. El conjunto de visitados evita ciclos.
func (idx *Index) allSubclasses(name string) []*ClassInfo {
	seen := map[*ClassInfo]bool{}
	var out []*ClassInfo
	var visit func(string)
	visit = func(n string) {
		for _, s := range idx.Children[n] {
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
				visit(s.Name)
			}
		}
	}
	visit(name)
	return out
}

func walk(n *sitter.Node, visit func(*sitter.Node)) {
	visit(n)
	for i := 0; i < int(n.ChildCount()); i++ {
		walk(n.Child(i), visit)
	}
}

func (idx *Index) parseFile(p *sitter.Parser, lg *language, file string) error {
	src, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	tree, err := p.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return err
	}
	defer tree.Close()

	if tree.RootNode().HasError() {
		idx.Errores = append(idx.Errores, file+": sintaxis inválida; el análisis puede estar incompleto")
	}

	imps := map[string]string{}
	var classes []*ClassInfo

	walk(tree.RootNode(), func(n *sitter.Node) {
		t := n.Type()
		switch {
		case lg.isClass[t]:
			if c := lg.extractClass(n, src, file); c != nil {
				c.Lang = lg.name
				classes = append(classes, c)
			}
		case lg.isImport[t]:
			for k, v := range lg.imports(n, src) {
				imps[k] = v
			}
		case lg.isIdent[t]:
			name := n.Content(src)
			idx.Mentions[name]++
			if kind := lg.usage(n, src); kind != "" {
				idx.addUso(name, kind)
			}
		}
	})

	for _, c := range classes {
		c.imports = imps
		idx.addClass(c)
	}
	idx.Archivos++
	return nil
}

// buildIndex recorre el proyecto y parsea cada archivo con el lenguaje que le corresponde.
func buildIndex(root string, langs []*language) (*Index, error) {
	parsers := map[*language]*sitter.Parser{}
	defer func() {
		for _, p := range parsers {
			p.Close()
		}
	}()

	idx := newIndex()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			idx.Errores = append(idx.Errores, fmt.Sprintf("%s: %v", p, err))
			return nil
		}
		if d.IsDir() {
			if p != root && skipDir[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		lg := langFor(p, langs)
		if lg == nil {
			return nil
		}
		par, ok := parsers[lg]
		if !ok {
			par = sitter.NewParser()
			par.SetLanguage(lg.grammar)
			parsers[lg] = par
		}
		if err := idx.parseFile(par, lg, p); err != nil {
			idx.Errores = append(idx.Errores, fmt.Sprintf("%s: %v", p, err))
		}
		return nil
	})
	return idx, err
}
