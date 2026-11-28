package main

import (
	"fmt"
	"os"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	if _, err := os.Stat(root); err != nil {
		fmt.Fprintln(os.Stderr, "❌", err)
		os.Exit(1)
	}

	reports, tree, env, walkDone := scan(root)

	fmt.Printf("🔍 Analizando %s\n\n", root)

	var st stats
	for r := range reports {
		if !st.add(r, root) {
			continue
		}
		fmt.Println(formatReport(r, root))
		fmt.Println()
	}

	<-walkDone
	fmt.Println("🌳 Estructura")
	fmt.Println()
	fmt.Print(renderTree(tree))
	fmt.Println(st.summary())
	fmt.Println(st.hiddenSummary())
	fmt.Println(env.summary())
}
