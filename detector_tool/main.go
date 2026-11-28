package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: go run . <directorio-del-proyecto>")
		os.Exit(1)
	}
	idx, err := buildIndex(os.Args[1], allLanguages())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	report(idx)
}

func report(idx *Index) {
	for _, c := range idx.Classes {
		defs := len(idx.Defs[c.Name])
		ambiguo := defs > 1
		fmt.Printf("clase %s [%s]  (%s:%d)\n", c.Name, c.Lang, c.File, c.Line)
		if ambiguo {
			fmt.Printf("  ⚠ ambiguo: %d definiciones con este nombre\n", defs)
		}

		fmt.Printf("  hereda de:  %s\n", describeBases(c))

		if ambiguo {
			// Con nombres repetidos, subclases y usos mezclarían clases distintas.
			fmt.Println("  subclases:  no calculadas (nombre repetido)")
			fmt.Println("  usos:       no calculados (nombre repetido)")
		} else {
			printSubclasesYUsos(idx, c)
		}

		fmt.Printf("  métodos:    %v\n", c.Methods)
		fmt.Printf("  tipo:       %s\n\n", describeTags(classify(c)))
	}

	fmt.Printf("archivos analizados: %d, clases: %d\n", idx.Archivos, len(idx.Classes))
	if len(idx.Errores) > 0 {
		fmt.Printf("errores (%d):\n", len(idx.Errores))
		for _, e := range idx.Errores {
			fmt.Println("  " + e)
		}
	}
}

func printSubclasesYUsos(idx *Index, c *ClassInfo) {
	subs := idx.allSubclasses(c.Name)
	direct := len(idx.Children[c.Name])
	fmt.Printf("  subclases:  %d (directas: %d)", len(subs), direct)
	if len(subs) > 0 {
		names := make([]string, 0, len(subs))
		for _, s := range subs {
			names = append(names, s.Name)
		}
		fmt.Printf(" %s", strings.Join(names, ", "))
	}
	fmt.Println()

	usos := idx.Usos[c.Name]
	total := 0
	var partes []string
	kinds := make([]string, 0, len(usos))
	for k := range usos {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		total += usos[k]
		partes = append(partes, fmt.Sprintf("%s %d", k, usos[k]))
	}
	otras := idx.Mentions[c.Name] - len(idx.Defs[c.Name]) - total
	if otras < 0 {
		otras = 0
	}
	if otras > 0 {
		partes = append(partes, fmt.Sprintf("otras menciones %d", otras))
	}
	if len(partes) == 0 {
		partes = []string{"sin usos"}
	}
	fmt.Printf("  usos:       %s\n", strings.Join(partes, ", "))
}

func describeBases(c *ClassInfo) string {
	if len(c.Bases) == 0 {
		return "(ninguna)"
	}
	var parts []string
	for _, b := range c.Bases {
		name := b.name
		if b.qual != "" {
			name = b.qual + "." + b.name
		}
		local := b.name
		if b.qual != "" {
			local = b.qual
		}
		if mod, ok := c.imports[firstSegment(local)]; ok {
			name += " (desde " + mod + ")"
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ", ")
}

func describeTags(ts []tag) string {
	if len(ts) == 0 {
		return "ninguna"
	}
	var parts []string
	for _, t := range ts {
		parts = append(parts, fmt.Sprintf("%s (%s: %s)", t.kind, t.fuente, t.detalle))
	}
	return strings.Join(parts, ", ")
}
