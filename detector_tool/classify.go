package main

import (
	"strings"
	"unicode"
)

// tag es una clasificación de la clase con su evidencia.
type tag struct {
	kind    string // "validación" o "proceso"
	fuente  string // "import" (confianza alta) o "nombre" (heurística)
	detalle string // módulo o palabra que dio la pista
}

var (
	validacionModulos = set("pydantic", "marshmallow", "attr", "attrs", "enum",
		"zod", "yup", "joi", "class-validator", "ajv", "cerberus")
	procesoModulos = set("threading", "multiprocessing", "concurrent", "asyncio",
		"worker_threads", "child_process", "cluster")

	// Bases conocidas aunque no se resuelva el import (p. ej. herencia de un archivo no indexado).
	basesConocidas = map[string]string{
		"BaseModel": "validación", "Enum": "validación", "IntEnum": "validación", "StrEnum": "validación",
		"Thread": "proceso", "Process": "proceso",
	}

	// Pistas por palabra completa, no por subcadena.
	pistasValidacion = set("validate", "validator", "validators", "schema", "valid", "check", "clean", "enum")
	pistasProceso    = set("run", "execute", "handle", "start", "thread", "process", "worker", "task")
)

// topModule: "pydantic.v1" -> "pydantic", "zod/v4" -> "zod", "." -> "".
func topModule(mod string) string {
	parts := strings.FieldsFunc(mod, func(r rune) bool { return r == '.' || r == '/' })
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// tokens separa un nombre en palabras: "BaseModel" -> [base model], "is_valid" -> [is valid].
func tokens(s string) []string {
	var b strings.Builder
	rs := []rune(s)
	for i, r := range rs {
		if i > 0 && unicode.IsUpper(r) && unicode.IsLower(rs[i-1]) {
			b.WriteRune(' ')
		}
		if unicode.IsLetter(r) {
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Fields(b.String())
}

// classify prioriza los imports resueltos y usa nombres solo como respaldo.
func classify(c *ClassInfo) []tag {
	var out []tag
	seen := map[string]bool{}
	add := func(t tag) {
		if !seen[t.kind] {
			seen[t.kind] = true
			out = append(out, t)
		}
	}

	for _, b := range c.Bases {
		local := b.name
		if b.qual != "" {
			local = b.qual
		}
		if mod, ok := c.imports[firstSegment(local)]; ok {
			top := topModule(mod)
			switch {
			case validacionModulos[top]:
				add(tag{"validación", "import", mod})
			case procesoModulos[top]:
				add(tag{"proceso", "import", mod})
			}
		}
		if k, ok := basesConocidas[b.name]; ok {
			add(tag{k, "nombre", b.name})
		}
	}

	var nombres []string
	for _, b := range c.Bases {
		nombres = append(nombres, b.name)
	}
	nombres = append(nombres, c.Methods...)
	for _, n := range nombres {
		for _, tok := range tokens(n) {
			if pistasValidacion[tok] {
				add(tag{"validación", "nombre", tok})
			}
			if pistasProceso[tok] {
				add(tag{"proceso", "nombre", tok})
			}
		}
	}
	return out
}

func firstSegment(s string) string {
	return strings.SplitN(s, ".", 2)[0]
}
