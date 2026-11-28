package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Componentes: separa las líneas de código de un archivo Python o JS en
// SDK + API, lógica y prints/logs. detector.go lo llama desde inspectFile,
// que ya tiene el código leído, así que no recorre el disco de nuevo.

// ccSDKPackages son los paquetes del SDK de Apify. Crawlee no está aquí: es una librería aparte.
var ccSDKPackages = map[string]map[string]bool{
	"python":     {"apify": true, "apify_client": true},
	"javascript": {"apify": true, "apify-client": true},
}

// ccAPILibPackages son las librerías que hacen llamadas HTTP/API; su bloque va junto al SDK.
var ccAPILibPackages = map[string]map[string]bool{
	"python":     {"httpx": true, "httpx2": true, "requests": true, "aiohttp": true, "fastapi": true},
	"javascript": {"axios": true, "got": true, "node-fetch": true},
}

// ccLoggerRe detecta prints, logs y la creación o uso de un logger.
// Tienen prioridad sobre el SDK.
var ccLoggerRe = map[string]*regexp.Regexp{
	"python":     regexp.MustCompile(`\bprint\s*\(|\blogg(?:ing|er)\b|\.(?:debug|info|warning|warn|error|exception|critical)\s*\(`),
	"javascript": regexp.MustCompile(`\bconsole\.\w+\s*\(|\blogger\b`),
}

// ccHTTPRe detecta endpoints y llamadas HTTP con get/post/put/delete, en cualquier
// framework: decoradores (@app.get), rutas (app.post('/x')) y clientes con URL literal.
// Exige ruta o URL literal en la llamada, así que dict.get("clave") no entra.
// fetch(...) global de JS también cuenta como llamada HTTP, como requests en Python.
var ccHTTPRe = regexp.MustCompile(
	`@[\w.]+\.(?:get|post|put|delete)\s*\(` +
		`|\.(?:get|post|put|delete)\s*\(\s*(?:f?['"` + "`" + `](?:/|https?://))` +
		`|(?:^|[^.\w$])fetch\s*\(`,
)

var (
	ccWordRe   = regexp.MustCompile(`\w+`)
	ccImportLn = regexp.MustCompile(`^\s*(from|import)\s`)

	ccPyFromRe   = regexp.MustCompile(`(?m)^\s*from\s+([\w.]+)\s+import\s+(.+)$`)
	ccPyImportRe = regexp.MustCompile(`(?m)^\s*import\s+([\w.]+)(?:\s+as\s+(\w+))?\s*$`)
	ccJSImportRe = regexp.MustCompile(`import\s+(.+?)\s+from\s+['"]([^'"]+)['"]`)

	ccPyAssignRe = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*=\s*([^=].*)$`)
	ccPyWithRe   = regexp.MustCompile(`^\s*(?:async\s+)?with\s+(.+?)\s+as\s+([A-Za-z_]\w*)\s*:`)
	ccJSDeclRe   = regexp.MustCompile(`^\s*(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=\s*(.+)$`)
	ccJSDestrRe  = regexp.MustCompile(`^\s*(?:const|let|var)\s+\{([^}]*)\}\s*=\s*(.+)$`)
)

// ccLine es una línea de código con su número original.
type ccLine struct {
	no   int
	text string
}

// components agrupa las líneas de un archivo por tipo.
type components struct {
	sdk, logic, logs []ccLine
}

type ccImportRef struct {
	pkg   string
	names []string
}

type ccAssignment struct {
	lhs []string
	rhs string
}

// componentsOf clasifica las líneas de código. lang llega como "python" o "js"
// (así lo nombra detector.go).
func componentsOf(lang, src string) *components {
	if lang == "js" {
		lang = "javascript"
	}
	src = strings.ReplaceAll(src, "\r", "")
	lines := strings.Split(src, "\n")
	for i, l := range lines {
		if ccIsComment(l, lang) {
			lines[i] = "" // conserva la numeración y evita que el comentario aporte nombres
		}
	}

	sdk, api := ccParseImports(strings.Join(lines, "\n"), lang)
	known := map[string]bool{}
	for n := range sdk {
		known[n] = true
	}
	for n := range api {
		known[n] = true
	}
	ccExpandKnown(lines, lang, known)
	sdkRes := ccNameRegexes(known)

	out := &components{}
	for i, l := range lines {
		if strings.TrimSpace(l) == "" || ccImportLn.MatchString(l) {
			continue
		}
		item := ccLine{no: i + 1, text: strings.TrimRight(l, " \t")}
		switch {
		case ccLoggerRe[lang].MatchString(l):
			out.logs = append(out.logs, item)
		case ccHTTPRe.MatchString(l), ccAnyMatch(l, sdkRes):
			out.sdk = append(out.sdk, item)
		default:
			out.logic = append(out.logic, item)
		}
	}
	return out
}

// writeComponent escribe un bloque de componentes con su título y sus líneas.
func writeComponent(b *strings.Builder, title string, lines []ccLine) {
	fmt.Fprintf(b, "\n      %s (%d):", title, len(lines))
	if len(lines) == 0 {
		b.WriteString("\n        (ninguno)")
		return
	}
	for _, l := range lines {
		fmt.Fprintf(b, "\n        %4d  %s", l.no, l.text)
	}
}

// ---------- imports ----------

var ccImportParsers = map[string]func(string) []ccImportRef{
	"python":     ccParsePyImports,
	"javascript": ccParseJSImports,
}

func ccParsePyImports(src string) []ccImportRef {
	var refs []ccImportRef
	for _, m := range ccPyFromRe.FindAllStringSubmatch(src, -1) {
		var names []string
		for _, n := range strings.Split(m[2], ",") {
			parts := strings.Split(strings.TrimSpace(n), " as ")
			names = append(names, strings.TrimSpace(parts[len(parts)-1]))
		}
		refs = append(refs, ccImportRef{pkg: strings.Split(m[1], ".")[0], names: names})
	}
	for _, m := range ccPyImportRe.FindAllStringSubmatch(src, -1) {
		name := m[2]
		if name == "" {
			name = strings.Split(m[1], ".")[0]
		}
		refs = append(refs, ccImportRef{pkg: strings.Split(m[1], ".")[0], names: []string{name}})
	}
	return refs
}

func ccParseJSImports(src string) []ccImportRef {
	var refs []ccImportRef
	for _, m := range ccJSImportRe.FindAllStringSubmatch(src, -1) {
		refs = append(refs, ccImportRef{pkg: m[2], names: ccWordRe.FindAllString(m[1], -1)})
	}
	return refs
}

// ccParseImports devuelve (nombres_sdk, nombres_api_libs).
func ccParseImports(src, lang string) (sdk, api map[string]bool) {
	sdk, api = map[string]bool{}, map[string]bool{}
	for _, r := range ccImportParsers[lang](src) {
		var dst map[string]bool
		switch {
		case ccSDKPackages[lang][r.pkg]:
			dst = sdk
		case ccAPILibPackages[lang][r.pkg]:
			dst = api
		default:
			continue
		}
		for _, n := range r.names {
			if n != "" && n != "as" {
				dst[n] = true
			}
		}
	}
	return sdk, api
}

// ---------- comentarios ----------

func ccIsComment(line, lang string) bool {
	prefix := map[string]string{"python": "#", "javascript": "//"}[lang]
	return prefix != "" && strings.HasPrefix(strings.TrimSpace(line), prefix)
}

// ---------- nombres ----------

func ccNameRegexes(names map[string]bool) []*regexp.Regexp {
	var res []*regexp.Regexp
	for n := range names {
		res = append(res, regexp.MustCompile(`\b`+regexp.QuoteMeta(n)+`\b`))
	}
	return res
}

func ccAnyMatch(s string, res []*regexp.Regexp) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func ccParseAssignments(lines []string, lang string) []ccAssignment {
	var out []ccAssignment
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if lang == "python" {
			if m := ccPyWithRe.FindStringSubmatch(l); m != nil {
				out = append(out, ccAssignment{lhs: []string{m[2]}, rhs: m[1]})
			} else if m := ccPyAssignRe.FindStringSubmatch(l); m != nil {
				out = append(out, ccAssignment{lhs: []string{m[1]}, rhs: m[2]})
			}
			continue
		}
		if m := ccJSDestrRe.FindStringSubmatch(l); m != nil {
			out = append(out, ccAssignment{lhs: ccWordRe.FindAllString(m[1], -1), rhs: m[2]})
		} else if m := ccJSDeclRe.FindStringSubmatch(l); m != nil {
			out = append(out, ccAssignment{lhs: []string{m[1]}, rhs: m[2]})
		}
	}
	return out
}

// ccExpandKnown añade a known los nombres asignados en líneas que usan nombres
// de los imports (un solo salto). No sigue la cadena: `response` derivado de
// `client`, o `url` derivado de `actor_input`, quedan fuera.
func ccExpandKnown(lines []string, lang string, known map[string]bool) {
	res := ccNameRegexes(known) // snapshot de los nombres de nivel 0
	for _, a := range ccParseAssignments(lines, lang) {
		if !ccAnyMatch(a.rhs, res) {
			continue
		}
		for _, n := range a.lhs {
			if n != "" {
				known[n] = true
			}
		}
	}
}
