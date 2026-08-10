package cmd

import (
	"flag"
	"fmt"
	"os"

	"mvnp/internal/npm"
)

func runSettings(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("settings command requires init or show")
	}

	switch args[0] {
	case "init":
		return runSettingsInit(args[1:])
	case "show":
		return runSettingsShow(args[1:])
	default:
		return fmt.Errorf("unknown settings command %q", args[0])
	}
}

func runSettingsInit(args []string) error {
	fs := flag.NewFlagSet("settings init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	global := fs.Bool("global", false, "create user global settings")

	if err := fs.Parse(args); err != nil {
		return err
	}

	target := "."
	if fs.NArg() > 0 {
		target = fs.Arg(0)
	}

	if *global {
		path, err := npm.GlobalSettingsPath()
		if err != nil {
			return err
		}
		if err := npm.SaveSettingsFile(path, npm.DefaultSettings()); err != nil {
			return err
		}
		fmt.Printf("created %s\n", path)
		return nil
	}

	path, err := npm.InitProjectSettings(target)
	if err != nil {
		return err
	}
	fmt.Printf("created %s\n", path)
	return nil
}

func runSettingsShow(args []string) error {
	fs := flag.NewFlagSet("settings show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

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
	printResolvedSettings(resolved)
	return nil
}
