package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// schemaLine es una clave de un JSON: su nivel de anidamiento, su nombre y su tipo de dato.
type schemaLine struct {
	depth int
	key   string
	typ   string
}

// jsonSchema devuelve las claves de un JSON, anidadas y con su tipo de dato.
// Si el contenido no es JSON válido devuelve error.
func jsonSchema(data []byte) ([]schemaLine, error) {
	var v any
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &v); err != nil {
		return nil, err
	}
	out := []schemaLine{}
	if m, ok := v.(map[string]any); ok {
		emitFields(&out, 0, []map[string]any{m})
	} else {
		emitField(&out, 0, "(raíz)", []any{v})
	}
	return out, nil
}

// jsonKind devuelve el tipo de dato de un valor JSON, en español.
func jsonKind(v any) string {
	switch v.(type) {
	case nil:
		return "nulo"
	case bool:
		return "booleano"
	case float64:
		return "número"
	case string:
		return "texto"
	case map[string]any:
		return "objeto"
	case []any:
		return "lista"
	}
	return "desconocido"
}

// emitField escribe una clave con su tipo y, si tiene hijos, sus claves anidadas.
// vals son los valores que esa clave toma en los objetos del mismo nivel.
func emitField(out *[]schemaLine, depth int, key string, vals []any) {
	var kinds []string
	var inst []map[string]any
	for _, v := range vals {
		k := jsonKind(v)
		if m, ok := v.(map[string]any); ok {
			inst = append(inst, m)
		}
		if list, ok := v.([]any); ok {
			var elems []string
			for _, e := range list {
				addUnique(&elems, jsonKind(e))
				if m, ok := e.(map[string]any); ok {
					inst = append(inst, m)
				}
			}
			if len(elems) > 0 {
				k = "lista de " + strings.Join(elems, " | ")
			}
		}
		addUnique(&kinds, k)
	}
	*out = append(*out, schemaLine{depth: depth, key: key, typ: strings.Join(kinds, " | ")})
	if len(inst) > 0 {
		emitFields(out, depth+1, inst)
	}
}

// emitFields escribe las claves (unidas) de uno o más objetos del mismo nivel, en orden alfabético.
func emitFields(out *[]schemaLine, depth int, objs []map[string]any) {
	vals := map[string][]any{}
	for _, m := range objs {
		for k, v := range m {
			vals[k] = append(vals[k], v)
		}
	}
	keys := make([]string, 0, len(vals))
	for k := range vals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		emitField(out, depth, k, vals[k])
	}
}

func addUnique(list *[]string, s string) {
	for _, x := range *list {
		if x == s {
			return
		}
	}
	*list = append(*list, s)
}

// jsonSummary es la línea del resumen con la cantidad de archivos JSON y sus claves.
func (s stats) jsonSummary() string {
	if s.jsonFiles == 0 {
		return ""
	}
	out := fmt.Sprintf("\n🧾 Archivos JSON: %d · %d claves en total", s.jsonFiles, s.jsonKeys)
	if s.jsonInvalid > 0 {
		out += fmt.Sprintf(" · ⚠️  %d inválidos", s.jsonInvalid)
	}
	return out
}
