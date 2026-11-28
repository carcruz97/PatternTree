package main

import (
        "fmt"
        "regexp"
        "slices"
        "strings"
)

// Complejidad: estima la profundidad de bucles anidados del cuerpo de una función.
// symbols.go llama a complexityOf con sus candidatos (cand) y usa explain y alert
// para el informe. El lenguaje llega como "python" o "js", igual que en el resto del proyecto.

var (
        pyLoopRe = regexp.MustCompile(`^\s*(?:async\s+)?(?:for|while)\b`)
        // Bucles con llaves: for, while y las llamadas funcionales que iteran.
        jsLoopRe = regexp.MustCompile(`\b(?:for|while)\b|\.(?:forEach|map|filter|reduce)\s*\(`)
        // for (const x of y): captura la variable (x) y el iterable (y).
        jsForOfRe = regexp.MustCompile(`\bfor\s*\(\s*(?:const|let|var)?\s*(\w+)\s+of\s+(\w+)\s*\)`)
        // Separa el texto en llaves y saltos de línea, que son los que cambian el estado.
        tokenLlavesRe = regexp.MustCompile(`[{}\n]`)
)

// signoLlave suma o resta nivel según la llave: el valor de un carácter ausente es 0.
var signoLlave = map[rune]int{'{': 1, '}': -1}

// jsLoop es un bucle abierto (llaves): profundidad a la que está y, si es un
// for...of, el nombre de su variable.
type jsLoop struct {
        depth int
        v     string
}

// complexityOf devuelve la profundidad máxima de bucles anidados del cuerpo de c.
// c.end es el fin de la firma: desde ahí empieza el cuerpo.
func complexityOf(src, lang string, c cand) int {
        if c.end <= 0 || c.end > len(src) {
                return 0
        }
        if lang == "python" {
                return pyLoopDepth(cuerpoPython(src, c))
        }
        return jsLoopDepth(cuerpoLlaves(src, c))
}

// cuerpoPython devuelve las líneas del cuerpo, que siguen mientras la sangría sea mayor
// que la de la declaración.
func cuerpoPython(src string, c cand) string {
        lineas := strings.Split(src[c.end:], "\n")[1:]
        fin := slices.IndexFunc(lineas, func(l string) bool {
                return !esVacia(l) && indentOf(l) <= c.indent
        })
        if fin < 0 {
                return strings.Join(lineas, "\n")
        }
        return strings.Join(lineas[:fin], "\n")
}

// cuerpoLlaves devuelve el cuerpo de una función con llaves o de una función flecha.
// Si la flecha tiene expresión en lugar de bloque, devuelve solo esa línea.
func cuerpoLlaves(src string, c cand) string {
        rest := src[c.end:]
        if src[c.end-1] == '{' {
                return cierreLlave(rest)
        }
        s := strings.TrimLeft(rest, " \t\r\n")
        if strings.HasPrefix(s, "{") {
                return cierreLlave(s[1:])
        }
        linea, _, _ := strings.Cut(rest, "\n")
        return linea
}

// cierreLlave devuelve el texto hasta la llave que cierra el bloque ya abierto.
func cierreLlave(rest string) string {
        nivel := 1
        for i, ch := range rest {
                nivel += signoLlave[ch]
                if nivel == 0 {
                        return rest[:i]
                }
        }
        return rest
}

func indentOf(l string) int {
        return len(l) - len(strings.TrimLeft(l, " \t"))
}

func esVacia(l string) bool {
        return strings.TrimSpace(l) == ""
}

// pyLoopDepth usa la indentación para saber qué bucles siguen abiertos. Como la pila
// queda ordenada por sangría, quitar todo lo que tenga sangría >= a la actual equivale a
// desapilar mientras la cima tenga sangría >= a la actual.
func pyLoopDepth(body string) int {
        var pila []int
        best := 0
        for _, l := range slices.DeleteFunc(strings.Split(body, "\n"), esVacia) {
                ind := indentOf(l)
                pila = slices.DeleteFunc(pila, func(i int) bool { return i >= ind })
                pila = apilarSi(pila, pyLoopRe.MatchString(l), ind)
                best = max(best, len(pila))
        }
        return best
}

// iteratesElementOf indica si el encabezado es un for...of que recorre la variable de un
// bucle que ya está abierto (p. ej. `for (const v of row)` dentro de `for (const row of rows)`).
// Ese bucle recorre los elementos de una fila, cuyo tamaño no depende de cuántas filas haya,
// así que no suma un nivel de complejidad.
func iteratesElementOf(pila []jsLoop, header string) bool {
        m := jsForOfRe.FindStringSubmatch(header)
        return m != nil && slices.ContainsFunc(pila, func(l jsLoop) bool { return l.v == m[2] })
}

// jsLoopDepth usa las llaves para saber qué bucles siguen abiertos. Cada llave o salto
// de línea cierra el segmento anterior, que es el encabezado del bloque que empieza.
func jsLoopDepth(body string) int {
        var pila []jsLoop
        best, depth, seg := 0, 0, 0
        for _, t := range tokenLlavesRe.FindAllStringIndex(body, -1) {
                switch body[t[0]] {
                case '{':
                        pila = abrirBucle(pila, body[seg:t[0]], depth)
                        depth++
                case '}':
                        depth--
                        pila = slices.DeleteFunc(pila, func(l jsLoop) bool { return l.depth >= depth })
                }
                best = max(best, len(pila))
                seg = t[0] + 1
        }
        return best
}

// abrirBucle apila el bloque si su encabezado es un bucle que no recorre elementos
// de un bucle abierto.
func abrirBucle(pila []jsLoop, header string, depth int) []jsLoop {
        if !jsLoopRe.MatchString(header) || iteratesElementOf(pila, header) {
                return pila
        }
        return append(pila, jsLoop{depth: depth, v: variableDe(header)})
}

// variableDe devuelve la variable de un for...of, o "" si el encabezado no es uno.
func variableDe(header string) string {
        m := jsForOfRe.FindStringSubmatch(header)
        if m == nil {
                return ""
        }
        return m[1]
}

// apilarSi agrega v a la pila solo si ok es verdadero.
func apilarSi[T any](pila []T, ok bool, v T) []T {
        if !ok {
                return pila
        }
        return append(pila, v)
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
