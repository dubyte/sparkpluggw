package main

import (
	"fmt"
	"os"

	"github.com/tkanos/go-dtree"
)

const filepathPos = 1

func main() {
	if len(os.Args) == 1 {
		fmt.Fprintf(os.Stderr, "showtree: missing operand\nTry 'showtree decisiontree.json'")
		os.Exit(1)
	}

	t := MustLoadDecisionTree(os.Args[filepathPos])

	fmt.Printf("tree:\n%s", t)
}

func MustLoadDecisionTree(filepath string) *dtree.Tree {
	fInfo, err := os.Stat(filepath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "showtree error: %s", err)
		os.Exit(1)
	}

	if fInfo.IsDir() {
		fmt.Fprintf(os.Stderr, "showtree error: the path is a directory")
		os.Exit(1)
	}

	data, err := os.ReadFile(filepath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "showtree error: %s", err)
		os.Exit(1)
	}

	dTree, err := dtree.LoadTree(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "showtree error: %s", err)
		os.Exit(1)
	}

	return dTree
}
