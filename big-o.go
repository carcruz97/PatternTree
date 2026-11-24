package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	pyLoopRe = regexp.MustCompile(`^\s*(?:async\s+)?(?:for|while)\b`)
	jsLoopRe = regexp.MustCompile(`\b(?:for|while)\b|\.(?:forEach|map|filter|reduce)\s*\(`)
	// for (const x of y): captura la variable (x) y el iterable (y).
	jsForOfRe = regexp.MustCompile(`\bfor\s*\(\s*(?:const|let|var)?\s*(\w+)\s+of\s+(\w+)\s*\)`)
)

// complexityOf devuelve la profundidad máxima de bucles anidados del cuerpo de c.
func complexityOf(src, lang string, c cand) int {
	if c.end <= 0 || c.end > len(src) {
		return 0
	}
	if lang == "python" {
		var body []string
		for _, l := range strings.Split(src[c.end:], "\n")[1:] {
			if strings.TrimSpace(l) != "" && indentOf(l) <= c.indent {
				break
			}
			body = append(body, l)
		}
		return pyLoopDepth(strings.Join(body, "\n"))
	}

	rest := src[c.end:]
	if src[c.end-1] != '{' { // arrow: cuerpo con llaves o expresión
		s := strings.TrimLeft(rest, " \t\r\n")
		if !strings.HasPrefix(s, "{") {
			line, _, _ := strings.Cut(rest, "\n")
			return jsLoopDepth(line)
		}
		rest = s[1:]
	}
	depth := 1
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return jsLoopDepth(rest[:i])
			}
		}
	}
	return jsLoopDepth(rest)
}

func indentOf(l string) int {
	return len(l) - len(strings.TrimLeft(l, " \t"))
}

// pyLoopDepth usa la indentación para saber qué bucles siguen abiertos.
func pyLoopDepth(body string) int {
	var stack []int
	best := 0
	for _, l := range strings.Split(body, "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		ind := indentOf(l)
		for len(stack) > 0 && stack[len(stack)-1] >= ind {
			stack = stack[:len(stack)-1]
		}
		if pyLoopRe.MatchString(l) {
			stack = append(stack, ind)
			best = max(best, len(stack))
		}
	}
	return best
}

// jsLoop es un bucle abierto: en qué profundidad está y, si es un for...of,
// el nombre de su variable.
type jsLoop struct {
	depth int
	v     string
}

// iteratesElementOf indica si el encabezado es un for...of que recorre la
// variable de un bucle que ya está abierto (p. ej. `for (const v of row)`
// dentro de `for (const row of rows)`). Ese bucle recorre los elementos de
// una fila, cuyo tamaño no depende de cuántas filas haya, así que no suma un
// nivel de complejidad.
func iteratesElementOf(stack []jsLoop, header string) bool {
	m := jsForOfRe.FindStringSubmatch(header)
	if m == nil {
		return false
	}
	for _, l := range stack {
		if l.v == m[2] {
			return true
		}
	}
	return false
}

// jsLoopDepth usa las llaves para saber qué bucles siguen abiertos.
func jsLoopDepth(body string) int {
	var stack []jsLoop
	best, depth, seg := 0, 0, 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '{':
			header := body[seg:i]
			if jsLoopRe.MatchString(header) && !iteratesElementOf(stack, header) {
				loop := jsLoop{depth: depth}
				if m := jsForOfRe.FindStringSubmatch(header); m != nil {
					loop.v = m[1]
				}
				stack = append(stack, loop)
				best = max(best, len(stack))
			}
			depth++
			seg = i + 1
		case '}':
			depth--
			for len(stack) > 0 && stack[len(stack)-1].depth >= depth {
				stack = stack[:len(stack)-1]
			}
			seg = i + 1
		case '\n':
			seg = i + 1
		}
	}
	return best
}

// explain dice la complejidad con un color y qué pasa si los datos crecen.
func explain(loops int) string {
	switch loops {
	case 0:
		return "🟢 O(1) constante: no depende de cuántos datos haya."
	case 1:
		return "🟢 O(n) lineal: con 10 veces más datos, tarda unas 10 veces más."
	case 2:
		return "🟠 O(n²) cuadrática: con 10 veces más datos, tarda unas 100 veces más."
	case 3:
		return "🔴 O(n³) cúbica: con 10 veces más datos, tarda unas 1000 veces más."
	}
	return fmt.Sprintf("🔴 O(n^%d): con 10 veces más datos, tarda unas 10^%d veces más.", loops, loops)
}

// alert devuelve una alerta con sugerencia para bucles anidados; vacío si no hace falta.
func alert(loops int) string {
	switch {
	case loops == 2:
		return "⚠️ Alerta: hay un bucle dentro de otro. Probá un mapa o un set para no recorrer todo de nuevo."
	case loops >= 3:
		return "🚨 Alerta: bucles muy anidados. Con muchos datos será muy lento; conviene reducir un nivel."
	}
	return ""
}
