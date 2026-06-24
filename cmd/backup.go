package cmd

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"mvnp/internal/maven"
)

func runBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	recursive := fs.Bool("recursive", false, "backup child module pom.xml files")
	backupDir := fs.String("backup-dir", "", "backup storage directory (default .mvnp/back)")
	label := fs.String("label", "", "optional backup label")
	list := fs.Bool("list", false, "list existing backups")

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

	if *list {
		manifests, err := maven.ListBackups(storeDir)
		if err != nil {
			return err
		}
		if len(manifests) == 0 {
			fmt.Printf("no backups found in %s\n", storeDir)
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "VERSION\tCREATED AT\tFILES\tLABEL")
		for _, item := range manifests {
			fmt.Fprintf(w, "v%d\t%s\t%d\t%s\n", item.Version, item.CreatedAt.Format("2006-01-02 15:04:05"), len(item.Files), item.Label)
		}
		return w.Flush()
	}

	report, err := maven.Backup(maven.BackupRequest{
		Root:      target,
		Recursive: *recursive,
		BackupDir: storeDir,
		Label:     *label,
		KeepCount: resolved.BackupKeepCount,
	})
	if err != nil {
		return err
	}

	fmt.Print(maven.FormatBackupReport(report))
	return nil
}
