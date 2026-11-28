package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

const queueSize = 256

type node struct {
	isDir    bool
	children map[string]*node
}

// scan recorre el proyecto, construye el árbol y analiza los archivos en paralelo.
// Devuelve el canal de reportes, el árbol (completo al cerrarse walkDone) y la señal de fin del recorrido.
func scan(root string) (<-chan fileReport, *node, *devEnv, <-chan struct{}) {
	paths := make(chan string, queueSize)
	reports := make(chan fileReport, queueSize)
	tree := &node{isDir: true, children: map[string]*node{}}
	walkDone := make(chan struct{})
	env := &devEnv{exts: map[string]int{}}

	go func() {
		defer close(walkDone)
		defer close(paths)
		walk(root, tree, env, paths)
	}()

	var workers sync.WaitGroup
	for range runtime.NumCPU() {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for p := range paths {
				reports <- inspectFile(p)
			}
		}()
	}
	go func() {
		workers.Wait()
		close(reports)
	}()

	return reports, tree, env, walkDone
}

// walk recorre el árbol, registra cada entrada y encola los archivos regulares.
// Los entornos de desarrollo no entran al árbol ni a la cola: se cuentan aparte.
func walk(root string, tree *node, env *devEnv, paths chan<- string) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  %v\n", err)
			return nil
		}
		if p == root {
			return nil
		}
		if d.IsDir() && isDevEnv(p, d.Name()) {
			env.add(root, p)
			return filepath.SkipDir
		}
		insert(tree, root, p, d.IsDir())
		if d.Type().IsRegular() {
			paths <- p
		}
		return nil
	})
}

// devEnvNames son carpetas de entornos y herramientas que se cuentan aparte.
var devEnvNames = map[string]bool{
	"venv": true, ".venv": true, "env": true, ".env": true, "__pypackages__": true,
	"node_modules": true, ".npm": true, ".pnpm": true, ".pnpm-store": true,
	".bun": true, ".uv": true, ".conda": true,
}

// isDevEnv dice si la carpeta es un entorno: por nombre o por marcas de venv y conda.
func isDevEnv(p, name string) bool {
	if devEnvNames[name] {
		return true
	}
	for _, marca := range []string{"pyvenv.cfg", "conda-meta"} {
		if _, err := os.Stat(filepath.Join(p, marca)); err == nil {
			return true
		}
	}
	return false
}

// devEnv acumula los entornos de desarrollo encontrados y sus archivos por extensión.
type devEnv struct {
	dirs  []string
	files int
	exts  map[string]int
}

// add cuenta los archivos de la carpeta de entorno sin analizarlos.
func (e *devEnv) add(root, dir string) {
	e.dirs = append(e.dirs, displayPath(root, dir))
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		e.files++
		ext := strings.ToLower(filepath.Ext(p))
		if ext == "" {
			ext = "(sin extensión)"
		}
		e.exts[ext]++
		return nil
	})
}

// summary arma la sección "Entorno de desarrollo" con cantidad de archivos y tipos.
func (e *devEnv) summary() string {
	if len(e.dirs) == 0 {
		return ""
	}
	exts := make([]string, 0, len(e.exts))
	for ext := range e.exts {
		exts = append(exts, ext)
	}
	sort.Slice(exts, func(i, j int) bool {
		if e.exts[exts[i]] != e.exts[exts[j]] {
			return e.exts[exts[i]] > e.exts[exts[j]]
		}
		return exts[i] < exts[j]
	})
	parts := make([]string, 0, len(exts))
	for _, ext := range exts {
		parts = append(parts, fmt.Sprintf("%s (%d)", ext, e.exts[ext]))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n🧰 Entorno de desarrollo: %d archivos en %d carpetas", e.files, len(e.dirs))
	for _, d := range e.dirs {
		fmt.Fprintf(&b, "\n    %s/", d)
	}
	fmt.Fprintf(&b, "\n    tipos: %s", strings.Join(parts, ", "))
	return b.String()
}

// insert agrega una entrada al árbol y crea los nodos intermedios que falten.
func insert(tree *node, root, p string, isDir bool) {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return
	}
	current := tree
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = current.child(part)
	}
	current.isDir = isDir
}

// child devuelve el hijo con ese nombre, creándolo si no existe.
func (n *node) child(name string) *node {
	if existing, ok := n.children[name]; ok {
		return existing
	}
	created := &node{children: map[string]*node{}}
	n.children[name] = created
	return created
}

// renderTree devuelve el árbol como texto, con ramas y carpetas marcadas con "/".
func renderTree(n *node) string {
	var b strings.Builder
	writeTree(&b, n, "")
	return b.String()
}

// writeTree escribe los hijos de n en b, ordenados por nombre.
func writeTree(b *strings.Builder, n *node, prefix string) {
	names := make([]string, 0, len(n.children))
	for name := range n.children {
		names = append(names, name)
	}
	sort.Strings(names)

	for i, name := range names {
		child := n.children[name]
		connector, indent := branchFor(i == len(names)-1)
		label := name
		if child.isDir {
			label += "/"
		}
		b.WriteString(prefix + connector + label + "\n")
		writeTree(b, child, prefix+indent)
	}
}

// branchFor devuelve el conector y la sangría según sea el último hijo o no.
func branchFor(isLast bool) (connector, indent string) {
	if isLast {
		return "└── ", "    "
	}
	return "├── ", "│   "
}
