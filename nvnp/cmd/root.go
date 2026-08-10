package cmd

import (
	"flag"
	"fmt"
	"os"
)

const usage = `nvnp - npm package.json upgrade tooling

Usage:
  nvnp <command> [options] [directory]

Commands:
  upgrade   Upgrade dependency versions in package.json
  list      List upgradeable packages in package.json
  backup    Create a versioned backup of package.json files
  restore   Restore package.json files from a backup
  settings  Manage nvnp settings (init/show)
  help      Show help

Run "nvnp <command> -h" for command-specific options.
`

func Execute() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "upgrade":
		if err := runUpgrade(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "list":
		if err := runList(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "settings":
		if err := runSettings(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "backup":
		if err := runBackup(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "restore":
		if err := runRestore(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

func changedFlag(fs interface{ Visit(func(*flag.Flag)) }, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
