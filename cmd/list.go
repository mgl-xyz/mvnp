package cmd

import (
	"flag"
	"fmt"
	"os"

	"mvnp/internal/maven"
)

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	recursive := fs.Bool("recursive", false, "list packages from child module pom.xml files")
	includeParent := fs.Bool("include-parent", false, "include parent version")
	pomPath := fs.String("pom", "", "list packages from a specific pom.xml")
	numbered := fs.Bool("numbered", false, "show selection numbers")

	if err := fs.Parse(args); err != nil {
		return err
	}

	target := "."
	if fs.NArg() > 0 {
		target = fs.Arg(0)
	}

	entries, err := maven.ListPackages(maven.ListPackagesRequest{
		Root:          target,
		Recursive:     *recursive,
		IncludeParent: *includeParent,
		POMPath:       *pomPath,
	})
	if err != nil {
		return err
	}

	fmt.Print(maven.FormatPackageList(entries, *numbered))
	return nil
}
