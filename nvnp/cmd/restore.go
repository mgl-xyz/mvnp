package cmd

import (
	"flag"
	"fmt"
	"os"

	"mvnp/internal/npm"
)

func runRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	version := fs.Int("version", 0, "backup version to restore (default latest)")
	backupDir := fs.String("backup-dir", "", "backup storage directory")

	if err := fs.Parse(args); err != nil {
		return err
	}

	target := "."
	if fs.NArg() > 0 {
		target = fs.Arg(0)
	}

	resolved, absTarget, err := resolveRuntimeConfig(target, npm.SettingsOverrides{
		BackupDir: *backupDir,
	})
	if err != nil {
		return err
	}
	storeDir, err := backupDirForProject(resolved, absTarget, *backupDir)
	if err != nil {
		return err
	}

	report, err := npm.Restore(npm.RestoreRequest{
		Root:      target,
		BackupDir: storeDir,
		Version:   *version,
	})
	if err != nil {
		return err
	}

	fmt.Print(npm.FormatRestoreReport(report))
	return nil
}
