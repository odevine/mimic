// Command validate checks a template's source folder and reports every problem it
// finds, which is stricter than the engine's loader: a field the contract does not
// have is an error here
//
//	validate <folder>
//	validate -schema template
//	validate -schema layout
//
// The first form prints each problem with the path to it and exits with 1 when
// there is an error. The second writes the JSON Schema of template.json, and the
// third the schema of a file in the layouts folder
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/odevine/mimic/engine/author"
)

func main() {
	os.Exit(run())
}

func run() int {
	args := os.Args[1:]
	var dir string
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		dir, args = args[0], args[1:]
	}
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	schema := fs.String("schema", "", `print a JSON Schema instead: "template" for template.json or "layout" for a layout file`)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if dir == "" && fs.NArg() == 1 {
		dir = fs.Arg(0)
	}

	if *schema != "" {
		var out []byte
		var err error
		switch *schema {
		case "template":
			out, err = author.SourceSchema()
		case "layout":
			out, err = author.LayoutSchema()
		default:
			fmt.Fprintf(os.Stderr, "validate: -schema is template or layout, not %q\n", *schema)
			return 2
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "validate:", err)
			return 1
		}
		os.Stdout.Write(out)
		return 0
	}
	if dir == "" || fs.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "usage: validate <folder>, or validate -schema template|layout")
		return 2
	}

	ds := author.Validate(os.DirFS(dir), author.Options{})
	for _, d := range ds {
		fmt.Println(d)
	}
	if ds.HasErrors() {
		return 1
	}
	if len(ds) == 0 {
		fmt.Println("ok")
	}
	return 0
}
