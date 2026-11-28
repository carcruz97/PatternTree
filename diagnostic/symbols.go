package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// param es un parámetro de una función o método, con su tipo si lo declara.
type param struct {
	name string
	typ  string
}

// symbol es una función, método o clase encontrada en el archivo.
type symbol struct {
	depth  int // nivel de anidamiento: 0 en el nivel superior, 1 dentro de una clase, etc.
	kind   string
	name   string
	params []param
	ret    string
	loops  int // profundidad máxima de bucles; en clases, la del método más costoso
}

// cand es un candidato sin validar; el contexto (clase o no) se decide después.
type cand struct {
	pos, indent int
	kind        string // "clase", "función" o "" si solo puede ser método
	name        string
	params      []param
	ret         string
	end         int // fin de la firma: aquí empieza el cuerpo
}

var (
	pyDefRe   = regexp.MustCompile(`(?m)^([ \t]*)(?:async[ \t]+)?def[ \t]+(\w+)[ \t]*\(([^)]*)\)[ \t]*(?:->[ \t]*([^:\n]+?))?[ \t]*:`)
	pyClassRe = regexp.MustCompile(`(?m)^([ \t]*)class[ \t]+(\w+)`)

	jsFuncRe   = regexp.MustCompile(`(?m)^([ \t]*)(?:export[ \t]+)?(?:default[ \t]+)?(?:async[ \t]+)?function[ \t]*\*?[ \t]*(\w+)[ \t]*(?:<[^>\n]*>)?[ \t]*\(([^)]*)\)[ \t]*(?::[ \t]*([^{\n]+?))?[ \t]*\{`)
	jsArrowRe  = regexp.MustCompile(`(?m)^([ \t]*)(?:export[ \t]+)?const[ \t]+(\w+)[ \t]*(?::[^=\n]+)?=[ \t]*(?:async[ \t]+)?\(([^)]*)\)[ \t]*(?::[ \t]*([^=\n{]+?))?[ \t]*=>`)
	jsClassRe  = regexp.MustCompile(`(?m)^([ \t]*)(?:export[ \t]+)?(?:default[ \t]+)?(?:abstract[ \t]+)?class[ \t]+(\w+)`)
	jsMethodRe = regexp.MustCompile(`(?m)^([ \t]+)(?:(?:public|private|protected|static|async|override|readonly)[ \t]+)*(\w+)[ \t]*(?:<[^>\n]*>)?[ \t]*\(([^)]*)\)[ \t]*(?::[ \t]*([^{\n]+?))?[ \t]*\{`)
)

// palabras que tienen forma de llamada seguida de { y no son métodos.
var keywords = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true,
	"with": true, "function": true, "return": true, "else": true, "do": true, "try": true,
}

var (
	pySkip = map[string]bool{"self": true, "cls": true}
	jsSkip = map[string]bool{"this": true}
)

// analyzeSymbols devuelve funciones, clases y métodos del archivo, con su nivel de anidamiento.
func analyzeSymbols(lang, src string) []symbol {
	var cs []cand
	if lang == "python" {
		cs = pyCandidates(src)
	} else {
		cs = jsCandidates(src)
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].pos < cs[j].pos })

	type open struct {
		indent int
		kind   string
	}
	var stack []open
	var out []symbol
	for _, c := range cs {
		if c.kind == "" && keywords[c.name] {
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].indent >= c.indent {
			stack = stack[:len(stack)-1]
		}
		inClass := len(stack) > 0 && stack[len(stack)-1].kind == "clase"

		kind := c.kind
		switch {
		case kind == "":
			if !inClass {
				continue
			}
			kind = "método"
		case kind == "función" && inClass:
			kind = "método"
		}

		loops := 0
		if kind != "clase" {
			loops = complexityOf(src, lang, c)
		}
		out = append(out, symbol{depth: len(stack), kind: kind, name: c.name, params: c.params, ret: c.ret, loops: loops})
		stack = append(stack, open{indent: c.indent, kind: kind})
	}
	for i := range out {
		if out[i].kind != "clase" {
			continue
		}
		for j := i + 1; j < len(out) && out[j].depth > out[i].depth; j++ {
			out[i].loops = max(out[i].loops, out[j].loops)
		}
	}
	return out
}

func pyCandidates(src string) []cand {
	var cs []cand
	for _, m := range pyDefRe.FindAllStringSubmatchIndex(src, -1) {
		g := groupsOf(src, m)
		cs = append(cs, cand{pos: m[0], end: m[1], indent: len(g[1]), kind: "función", name: g[2],
			params: paramList(g[3], pySkip), ret: g[4]})
	}
	for _, m := range pyClassRe.FindAllStringSubmatchIndex(src, -1) {
		g := groupsOf(src, m)
		cs = append(cs, cand{pos: m[0], indent: len(g[1]), kind: "clase", name: g[2]})
	}
	return cs
}

func jsCandidates(src string) []cand {
	var cs []cand
	for _, m := range jsFuncRe.FindAllStringSubmatchIndex(src, -1) {
		g := groupsOf(src, m)
		cs = append(cs, cand{pos: m[0], end: m[1], indent: len(g[1]), kind: "función", name: g[2],
			params: paramList(g[3], jsSkip), ret: g[4]})
	}
	for _, m := range jsArrowRe.FindAllStringSubmatchIndex(src, -1) {
		g := groupsOf(src, m)
		cs = append(cs, cand{pos: m[0], end: m[1], indent: len(g[1]), kind: "función", name: g[2],
			params: paramList(g[3], jsSkip), ret: g[4]})
	}
	for _, m := range jsClassRe.FindAllStringSubmatchIndex(src, -1) {
		g := groupsOf(src, m)
		cs = append(cs, cand{pos: m[0], indent: len(g[1]), kind: "clase", name: g[2]})
	}
	for _, m := range jsMethodRe.FindAllStringSubmatchIndex(src, -1) {
		g := groupsOf(src, m)
		cs = append(cs, cand{pos: m[0], end: m[1], indent: len(g[1]), name: g[2],
			params: paramList(g[3], jsSkip), ret: g[4]})
	}
	return cs
}

// groupsOf devuelve el texto de cada grupo de una coincidencia; vacío si el grupo no participó.
func groupsOf(src string, m []int) []string {
	g := make([]string, len(m)/2)
	for i := range g {
		if m[2*i] >= 0 {
			g[i] = src[m[2*i]:m[2*i+1]]
		}
	}
	return g
}

// paramList separa la lista de parámetros, ignorando los que no cuentan (self, this, * y /).
func paramList(raw string, skip map[string]bool) []param {
	var out []param
	for _, part := range splitTop(raw) {
		if strings.TrimSpace(part) == "" {
			continue
		}
		p := parseParam(part)
		if p.name == "" || skip[p.name] || p.name == "*" || p.name == "/" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// parseParam saca nombre y tipo de "nombre: tipo = valor"; sin tipo se marca como "sin tipo".
func parseParam(s string) param {
	s = strings.TrimSpace(s)
	if i := topLevel(s, '='); i >= 0 {
		s = s[:i]
	}
	typ := "sin tipo"
	if i := topLevel(s, ':'); i >= 0 {
		if t := strings.TrimSpace(s[i+1:]); t != "" {
			typ = t
		}
		s = s[:i]
	}
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "?"))
	for _, mod := range []string{"private ", "public ", "protected ", "readonly "} {
		name = strings.TrimPrefix(name, mod)
	}
	return param{name: name, typ: typ}
}

// topLevel devuelve la posición de c fuera de (), [] y {}, o -1. Ignora el "=" de "=>".
func topLevel(s string, c byte) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		default:
			if s[i] == c && depth == 0 && !(c == '=' && i+1 < len(s) && s[i+1] == '>') {
				return i
			}
		}
	}
	return -1
}

// splitTop parte por comas que no están dentro de (), [] o {}.
func splitTop(s string) []string {
	var parts []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

// String muestra el símbolo con su nivel de sangría, sus parámetros y su salida.
func (s symbol) String() string {
	icons := map[string]string{"clase": "🏛️", "función": "🔧", "método": "⚙️"}
	pad := "      " + strings.Repeat("    ", s.depth)

	var b strings.Builder
	fmt.Fprintf(&b, "%s%s %s %s", pad, icons[s.kind], s.kind, s.name)
	fmt.Fprintf(&b, "\n%s    %s", pad, explain(s.loops))
	if a := alert(s.loops); a != "" {
		fmt.Fprintf(&b, "\n%s    %s", pad, a)
	}
	if s.kind == "clase" {
		return b.String()
	}

	fmt.Fprintf(&b, "\n%s    %s", pad, countText(len(s.params)))
	if len(s.params) > 0 {
		parts := make([]string, len(s.params))
		for i, p := range s.params {
			parts[i] = p.name + ": " + p.typ
		}
		b.WriteString(": " + strings.Join(parts, ", "))
	}
	ret := s.ret
	if ret == "" {
		ret = "sin tipo"
	}
	fmt.Fprintf(&b, "\n%s    devuelve: %s", pad, ret)
	return b.String()
}

func countText(n int) string {
	if n == 1 {
		return "1 parámetro"
	}
	return fmt.Sprintf("%d parámetros", n)
}

// parametroJSON es un parámetro con su nombre y su tipo de dato.
type parametroJSON struct {
	Nombre string `json:"nombre"`
	Tipo   string `json:"tipo"`
}

// simboloJSON refleja lo que muestra String: las clases no llevan parámetros ni devuelve.
// Los elementos anidados (métodos de una clase, funciones dentro de funciones) van en "hijos".
type simboloJSON struct {
	Tipo       string           `json:"tipo"`
	Nombre     string           `json:"nombre"`
	Bucles     int              `json:"bucles"`
	Parametros *[]parametroJSON `json:"parametros,omitempty"`
	Devuelve   *string          `json:"devuelve,omitempty"`
	Hijos      []simboloJSON    `json:"hijos,omitempty"`
}

func (s symbol) toJSON() simboloJSON {
	node := simboloJSON{Tipo: s.kind, Nombre: s.name, Bucles: s.loops}
	if s.kind != "clase" {
		params := make([]parametroJSON, 0, len(s.params))
		for _, p := range s.params {
			params = append(params, parametroJSON{Nombre: p.name, Tipo: p.typ})
		}
		ret := s.ret
		if ret == "" {
			ret = "sin tipo"
		}
		node.Parametros = &params
		node.Devuelve = &ret
	}
	return node
}

// buildTree convierte la lista plana (en orden, con depth) en un árbol anidado.
func buildTree(syms []symbol, i *int, depth int) []simboloJSON {
	var out []simboloJSON
	for *i < len(syms) && syms[*i].depth >= depth {
		s := syms[*i]
		*i++
		node := s.toJSON()
		node.Hijos = buildTree(syms, i, s.depth+1)
		out = append(out, node)
	}
	return out
}

// encodeJSON escribe v como JSON con sangría, sin escapar <, > ni &.
func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// symbolsTree convierte los símbolos planos de un archivo en el árbol JSON.
func symbolsTree(syms []symbol) []simboloJSON {
	i := 0
	tree := buildTree(syms, &i, 0)
	if tree == nil {
		tree = []simboloJSON{}
	}
	return tree
}

// symbolsJSON analiza el código y devuelve los símbolos en JSON anidado.
func symbolsJSON(lang, src string) ([]byte, error) {
	return encodeJSON(symbolsTree(analyzeSymbols(lang, src)))
}

// reportJSON es el JSON de un archivo analizado: su ruta y sus símbolos.
type reportJSON struct {
	Archivo  string        `json:"archivo"`
	Simbolos []simboloJSON `json:"simbolos"`
}

// reportsJSON arma el JSON de los archivos analizados (los que tienen símbolos).
func reportsJSON(reports []fileReport, root string) ([]byte, error) {
	out := []reportJSON{}
	for _, r := range reports {
		if r.err != nil || !r.analyzed {
			continue
		}
		out = append(out, reportJSON{Archivo: displayPath(root, r.path), Simbolos: symbolsTree(r.symbols)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Archivo < out[j].Archivo })
	return encodeJSON(out)
}
