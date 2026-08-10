package cmd

import (
	"flag"
	"fmt"
	"os"

	"mvnp/internal/npm"
)

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	numbered := fs.Bool("numbered", false, "show numbered list for selection")
	recursive := fs.Bool("recursive", false, "include child package.json files")
	packagePath := fs.String("package", "", "use a specific package.json")
	depRaw := fs.String("dep", "", "dependency sections: prod,dev,peer,optional")

	if err := fs.Parse(args); err != nil {
		return err
	}

	target := "."
	if fs.NArg() > 0 {
		target = fs.Arg(0)
	}

	resolved, _, err := resolveRuntimeConfig(target, npm.SettingsOverrides{})
	if err != nil {
		return err
	}

	depSections, err := parseDepSections(*depRaw, resolved.DepSections)
	if err != nil {
		return err
	}

	entries, err := npm.ListPackages(npm.ListPackagesRequest{
		Root:        target,
		Recursive:   *recursive,
		PackagePath: *packagePath,
		DepSections: depSections,
	})
	if err != nil {
		return err
	}

	fmt.Print(npm.FormatPackageList(entries, *numbered))
	return nil
}
