package cmd

import (
	"flag"
	"fmt"
	"os"

	"mvnp/internal/maven"
)

func runRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	backupDir := fs.String("backup-dir", "", "backup storage directory (default .mvnp/back)")
	version := fs.Int("version", 0, "backup version to restore (default latest)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	target := "."
	if fs.NArg() > 0 {
		target = fs.Arg(0)
	}

	resolved, absTarget, err := resolveRuntimeConfig(target, maven.SettingsOverrides{
		BackupDir: *backupDir,
	})
	if err != nil {
		return err
	}
	storeDir, err := backupDirForProject(resolved, absTarget, *backupDir)
	if err != nil {
		return err
	}

	report, err := maven.Restore(maven.RestoreRequest{
		Root:      target,
		BackupDir: storeDir,
		Version:   *version,
	})
	if err != nil {
		return err
	}

	fmt.Print(maven.FormatRestoreReport(report))
	return nil
}
