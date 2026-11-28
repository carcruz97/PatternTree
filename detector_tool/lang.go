package main

import (
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// language describe lo que cambia entre lenguajes: la gramática, los tipos de
// nodo relevantes y cómo extraer clases, imports y usos. Todo lo demás
// (recorrido, índice, herencia transitiva, reporte) es común.
type language struct {
	name     string
	exts     []string
	grammar  *sitter.Language
	isClass  map[string]bool // nodos que son definiciones de clase
	isImport map[string]bool // nodos de import
	isIdent  map[string]bool // nodos que representan un nombre (referencia o definición)

	extractClass func(n *sitter.Node, src []byte, file string) *ClassInfo
	imports      func(n *sitter.Node, src []byte) map[string]string // nombre local -> módulo
	usage        func(n *sitter.Node, src []byte) string            // "" si no es un uso clasificable
}

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func allLanguages() []*language {
	return []*language{pythonLang, javascriptLang, typescriptLang, tsxLang}
}

var pythonLang = &language{
	name:         "python",
	exts:         []string{".py"},
	grammar:      python.GetLanguage(),
	isClass:      set("class_definition"),
	isImport:     set("import_statement", "import_from_statement"),
	isIdent:      set("identifier"),
	extractClass: pyClass,
	imports:      pyImports,
	usage:        pyUsage,
}

var javascriptLang = &language{
	name:         "javascript",
	exts:         []string{".js", ".jsx", ".mjs", ".cjs"},
	grammar:      javascript.GetLanguage(),
	isClass:      set("class_declaration"),
	isImport:     set("import_statement"),
	isIdent:      set("identifier", "type_identifier"),
	extractClass: jsClass,
	imports:      jsImports,
	usage:        jsUsage,
}

var typescriptLang = &language{
	name:         "typescript",
	exts:         []string{".ts", ".mts", ".cts"},
	grammar:      typescript.GetLanguage(),
	isClass:      set("class_declaration", "abstract_class_declaration"),
	isImport:     set("import_statement"),
	isIdent:      set("identifier", "type_identifier"),
	extractClass: jsClass,
	imports:      jsImports,
	usage:        jsUsage,
}

// TSX necesita su propia gramática: <T>x no es válido ahí.
var tsxLang = &language{
	name:         "tsx",
	exts:         []string{".tsx"},
	grammar:      tsx.GetLanguage(),
	isClass:      set("class_declaration", "abstract_class_declaration"),
	isImport:     set("import_statement"),
	isIdent:      set("identifier", "type_identifier"),
	extractClass: jsClass,
	imports:      jsImports,
	usage:        jsUsage,
}

// langFor devuelve el lenguaje del archivo, o nil si no se analiza.
func langFor(path string, langs []*language) *language {
	if strings.HasSuffix(path, ".d.ts") {
		return nil
	}
	ext := filepath.Ext(path)
	for _, lg := range langs {
		for _, e := range lg.exts {
			if e == ext {
				return lg
			}
		}
	}
	return nil
}

var skipDir = set(
	".git", ".venv", "venv", "env", "__pycache__", "site-packages",
	"node_modules", "dist", "build", ".next", "coverage",
)
