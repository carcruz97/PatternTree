package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const headerBytes = 512

var extensionLabels = map[string]string{
	".csv":    "📊 CSV",
	".py":     "🐍 Python",
	".pkl":    "🥒 Pickle",
	".pickle": "🥒 Pickle",
	".joblib": "📦 Joblib",
	".ts":     "🔷 TypeScript",
	".json":   "🧾 JSON",
	".md":     "📝 Markdown",
	".js":     "📜 JavaScript",
	".mjs":    "📜 JavaScript",
	".cjs":    "📜 JavaScript",
	".jsx":    "⚛️ JSX",
	".tsx":    "⚛️ TSX",
}

// importLangs indica qué extensiones se analizan en busca de imports, y con qué reglas.
var importLangs = map[string]string{
	".py":  "python",
	".js":  "js",
	".mjs": "js",
	".cjs": "js",
	".jsx": "js",
	".ts":  "js",
	".tsx": "js",
}

var (
	identRe = regexp.MustCompile(`[A-Za-z_$][A-Za-z0-9_$]*`)

	pyImportRe = regexp.MustCompile(`(?m)^[ \t]*import[ \t]+([^\n#]+)`)
	pyFromRe   = regexp.MustCompile(`(?m)^[ \t]*from[ \t]+([\w.]+)[ \t]+import[ \t]+(\([^)]*\)|[^\n#]+)`)

	jsImportRe  = regexp.MustCompile(`\bimport\s+(?:type\s+)?([^;'"]*?)\s*from\s*['"]([^'"]+)['"]\s*;?`)
	jsSideRe    = regexp.MustCompile(`\bimport\s*['"]([^'"]+)['"]\s*;?`)
	jsRequireRe = regexp.MustCompile(`(?:const|let|var)\s+([\w$]+|\{[^}]*\})\s*=\s*require\(\s*['"]([^'"]+)['"]\s*\)`)
)

// dep es un nombre que un archivo importa.
type dep struct {
	module string // módulo de origen, por ejemplo "numpy" o "pandas.DataFrame"
	name   string // nombre con el que el archivo lo usa; vacío o "*" si no se puede verificar
	used   bool
}

// String muestra el módulo y, si hay alias distinto, el nombre local.
func (d dep) String() string {
	if d.name == "" {
		return d.module
	}
	if d.module == d.name || strings.HasSuffix(d.module, "."+d.name) {
		return d.module
	}
	return d.module + " → " + d.name
}

// line arma la línea del reporte con el estado del import.
func (d dep) line() string {
	switch {
	case d.name == "" || d.name == "*":
		return "❔ " + d.String() + " (no verificable)"
	case d.used:
		return "✅ " + d.String()
	default:
		return "❌ " + d.String() + " (no usado)"
	}
}

type fileReport struct {
	path       string
	kind       string
	analyzed   bool
	deps       []dep
	symbols    []symbol
	components *components
	schema     []schemaLine
	schemaErr  error
	err        error
}

// isInteresting dice si la extensión es una que el informe muestra.
func isInteresting(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	_, labeled := extensionLabels[ext]
	_, analyzed := importLangs[ext]
	return labeled || analyzed
}

// stats acumula el conteo de archivos y los que van al resumen, agrupados por extensión.
type stats struct {
	total, failed, hidden            int
	jsonFiles, jsonInvalid, jsonKeys int
	skipped                          map[string][]string
}

// add registra un reporte; devuelve false si el archivo no va al informe y solo queda en el resumen.
func (s *stats) add(r fileReport, root string) bool {
	s.total++
	if r.err != nil {
		s.failed++
		return true
	}
	if strings.EqualFold(filepath.Ext(r.path), ".json") {
		s.jsonFiles++
		s.jsonKeys += len(r.schema)
		if r.schemaErr != nil {
			s.jsonInvalid++
		}
	}
	if isInteresting(r.path) {
		return true
	}
	if s.skipped == nil {
		s.skipped = map[string][]string{}
	}
	ext := strings.ToLower(filepath.Ext(r.path))
	if ext == "" {
		ext = "(sin extensión)"
	}
	s.skipped[ext] = append(s.skipped[ext], displayPath(root, r.path))
	s.hidden++
	return false
}

// hiddenSummary lista los archivos ocultos del informe, agrupados por extensión.
func (s stats) hiddenSummary() string {
	if s.hidden == 0 {
		return ""
	}
	exts := make([]string, 0, len(s.skipped))
	for e := range s.skipped {
		exts = append(exts, e)
	}
	sort.Strings(exts)

	var b strings.Builder
	fmt.Fprintf(&b, "\n📦 Otros archivos (no mostrados): %d", s.hidden)
	for _, e := range exts {
		names := s.skipped[e]
		fmt.Fprintf(&b, "\n    %s (%d): %s", e, len(names), strings.Join(names, ", "))
	}
	return b.String()
}

// summary arma la línea final con el resumen del análisis.
func (s stats) summary() string {
	out := fmt.Sprintf("\n✅ %d archivos analizados", s.total-s.failed)
	if s.failed > 0 {
		out += fmt.Sprintf(" · ⚠️  %d con error", s.failed)
	}
	out += s.jsonSummary()
	return out
}

// formatReport arma el bloque de un archivo, o el aviso si no pudo leerse.
func formatReport(r fileReport, root string) string {
	name := displayPath(root, r.path)
	if r.err != nil {
		return fmt.Sprintf("⚠️  %s\n    No se pudo leer: %v", name, r.err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "📄 %s\n    Tipo:    %s", name, r.kind)
	if r.schemaErr != nil {
		fmt.Fprintf(&b, "\n    Estructura: ❌ JSON inválido: %v", r.schemaErr)
	} else if r.schema != nil {
		fmt.Fprintf(&b, "\n    Estructura (%d claves):", len(r.schema))
		if len(r.schema) == 0 {
			b.WriteString("\n      (sin claves)")
		}
		for _, l := range r.schema {
			fmt.Fprintf(&b, "\n      %s%s: %s", strings.Repeat("  ", l.depth), l.key, l.typ)
		}
	}
	if r.analyzed {
		b.WriteString("\n    Imports:")
		if len(r.deps) == 0 {
			b.WriteString("\n      (ninguno)")
		}
		for _, d := range r.deps {
			b.WriteString("\n      " + d.line())
		}

		b.WriteString("\n    Símbolos:")
		if len(r.symbols) == 0 {
			b.WriteString("\n      (ninguno)")
		}
		for _, s := range r.symbols {
			b.WriteString("\n" + s.String())
		}


		b.WriteString("\n    Componentes:")
		writeComponent(&b, "🔌 SDK + API", r.components.sdk)
		writeComponent(&b, "🧠 Lógica", r.components.logic)
		writeComponent(&b, "🖨️ Prints y logs", r.components.logs)
	}
	return b.String()
}

// displayPath muestra la ruta relativa a la carpeta analizada, más corta de leer.
func displayPath(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return rel
}

// inspectFile abre el archivo, obtiene su tipo y, si es código, sus imports; lo cierra siempre.
func inspectFile(p string) (r fileReport) {
	r.path = p
	if !isInteresting(p) {
		return r // no se abre: solo se agrupa por extensión
	}

	f, err := os.Open(p)
	if err != nil {
		r.err = err
		return r
	}
	defer func() { r.err = errors.Join(r.err, f.Close()) }()

	reader := bufio.NewReader(f)
	head, peekErr := reader.Peek(headerBytes)
	if !isReadOK(peekErr) {
		r.err = peekErr
		return r
	}

	r.kind = classify(head, p)
	if strings.EqualFold(filepath.Ext(p), ".json") {
		src, err := io.ReadAll(reader)
		if err != nil {
			r.err = err
			return r
		}
		r.schema, r.schemaErr = jsonSchema(src)
	}
	if lang, ok := importLangs[strings.ToLower(filepath.Ext(p))]; ok {
		src, err := io.ReadAll(reader)
		if err != nil {
			r.err = err
			return r
		}
		r.analyzed = true
		r.deps = analyzeImports(lang, string(src))
		r.symbols = analyzeSymbols(lang, string(src))
		r.components = componentsOf(lang, string(src))
	}
	return r
}

// isReadOK acepta nil y el fin de archivo, que es normal en archivos más cortos que la cabecera.
func isReadOK(err error) bool {
	return err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

// classify decide el tipo solo por la extensión; sin detección de contenido ni de binarios.
func classify(head []byte, p string) string {
	if len(head) == 0 {
		return "📭 Vacío"
	}
	if label, ok := extensionLabels[strings.ToLower(filepath.Ext(p))]; ok {
		return label
	}
	return "❓ " + filepath.Ext(p)
}

// analyzeImports extrae los imports del código y marca cuáles aparecen usados en el resto del archivo.
func analyzeImports(lang, src string) []dep {
	var deps []dep
	if lang == "python" {
		deps, src = pyDeps(src)
	} else {
		deps, src = jsDeps(src)
	}

	used := map[string]bool{}
	for _, id := range identRe.FindAllString(src, -1) {
		used[id] = true
	}
	for i := range deps {
		deps[i].used = used[deps[i].name]
	}
	return deps
}

// pyDeps devuelve los imports de Python y el código sin esas líneas.
func pyDeps(src string) ([]dep, string) {
	var deps []dep

	for _, m := range pyImportRe.FindAllStringSubmatch(src, -1) {
		for _, part := range strings.Split(m[1], ",") {
			mod, alias, found := strings.Cut(strings.TrimSpace(part), " as ")
			mod = strings.TrimSpace(mod)
			if mod == "" {
				continue
			}
			name := strings.SplitN(mod, ".", 2)[0]
			if found {
				name = strings.TrimSpace(alias)
			}
			deps = append(deps, dep{module: mod, name: name})
		}
	}

	for _, m := range pyFromRe.FindAllStringSubmatch(src, -1) {
		names := strings.Trim(m[2], "()")
		for _, part := range strings.Split(names, ",") {
			n, alias, found := strings.Cut(strings.TrimSpace(part), " as ")
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			name := n
			if found {
				name = strings.TrimSpace(alias)
			}
			deps = append(deps, dep{module: m[1] + "." + n, name: name})
		}
	}

	src = pyFromRe.ReplaceAllString(src, "")
	src = pyImportRe.ReplaceAllString(src, "")
	return deps, src
}

// jsDeps devuelve los imports de JS/TS y el código sin esas líneas.
func jsDeps(src string) ([]dep, string) {
	var deps []dep

	add := func(clause, module string) {
		names := clauseNames(clause)
		if len(names) == 0 {
			deps = append(deps, dep{module: module})
			return
		}
		for _, n := range names {
			deps = append(deps, dep{module: module, name: n})
		}
	}

	for _, m := range jsImportRe.FindAllStringSubmatch(src, -1) {
		add(m[1], m[2])
	}
	for _, m := range jsSideRe.FindAllStringSubmatch(src, -1) {
		deps = append(deps, dep{module: m[1]})
	}
	for _, m := range jsRequireRe.FindAllStringSubmatch(src, -1) {
		add(m[1], m[2])
	}

	src = jsImportRe.ReplaceAllString(src, "")
	src = jsSideRe.ReplaceAllString(src, "")
	src = jsRequireRe.ReplaceAllString(src, "")
	return deps, src
}

var clauseReplacer = strings.NewReplacer("{", ",", "}", ",", ":", " as ")

// clauseNames saca los nombres locales de una cláusula de import: default, * as ns, { a, b as c }.
func clauseNames(clause string) []string {
	var names []string
	for _, part := range strings.Split(clauseReplacer.Replace(clause), ",") {
		part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "type "))
		if part == "" {
			continue
		}
		if _, alias, found := strings.Cut(part, " as "); found {
			part = strings.TrimSpace(alias)
		}
		names = append(names, part)
	}
	return names
}
