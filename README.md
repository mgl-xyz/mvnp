# mvnp

Enhanced Maven tooling written in Go. **mvnp** helps you safely upgrade dependency versions in `pom.xml` files, with versioned backups and restore support.

[中文文档](README_zh.md)

## Features

- **Upgrade** dependency and plugin versions in `pom.xml`
- **Backup** `pom.xml` files with incremental version numbers
- **Restore** any previous backup (latest by default)
- **Auto-backup** before every upgrade (enabled by default)
- Policies aligned with [Maven Versions Plugin](https://www.mojohaus.org/versions/versions-maven-plugin/)
- Query versions from Maven Central (custom repository URL supported)

## Installation

Requires Go 1.21+.

```bash
git clone <repository-url>
cd mvnp
go build -o mvnp .
```

Optionally install into your `PATH`:

```bash
go install .
```

## Quick Start

```bash
# Preview upgrades in the current project
mvnp upgrade --dry-run

# Upgrade dependencies (auto-backup enabled by default)
mvnp upgrade

# Manually create a backup
mvnp backup

# List all backups
mvnp backup --list

# Restore the latest backup
mvnp restore

# Restore a specific backup version
mvnp restore --version 1
```

## Commands

### `upgrade`

Upgrade dependency versions in the current directory or a specified path.

```bash
mvnp upgrade [options] [directory]
```

**Policies**

| Policy | Description | Aliases |
|--------|-------------|---------|
| `latest-releases` | Upgrade to the latest release version (default) | `release`, `stable` |
| `latest-snapshots` | Upgrade SNAPSHOT dependencies to the latest SNAPSHOT | `snapshot` |
| `latest-versions` | Upgrade to the latest available version | `latest` |
| `next-releases` | Upgrade to the next newer release | |
| `next-snapshots` | Upgrade to the next newer SNAPSHOT | |
| `next-versions` | Upgrade to the next newer version | `next` |
| `releases` | Replace SNAPSHOT versions with the latest release | `use-releases` |

**Common options**

| Option | Default | Description |
|--------|---------|-------------|
| `-policy` | `latest-releases` | Upgrade policy |
| `-recursive` | `false` | Process child module `pom.xml` files |
| `-dry-run` | `false` | Preview without writing files |
| `-no-backup` | `false` | Skip automatic backup before upgrade |
| `-backup-dir` | `.mvnp/back` | Backup storage directory |
| `-only` | | Only upgrade one `groupId:artifactId` |
| `-include` | | Only upgrade listed `groupId:artifactId` |
| `-ignore` | | Skip listed `groupId:artifactId` |
| `-select` | `false` | Interactively choose packages to upgrade |
| `-ignore-select` | `false` | Interactively choose packages to ignore |
| `-pom` | | Use a specific `pom.xml` for list/select |
| `-include-parent` | `false` | Also upgrade parent version |
| `-quiet` | `false` | Disable progress output |
| `-verbose` | `false` | Show detailed multi-line progress |
| `-summary` | `false` | Print final summary report to stdout |

Default progress output is compact and docker pull-like:

```
Checking 45 entries (32 packages)

com.fasterxml.jackson:jackson-bom: Resolving [==========          ]  38% 12/32
com.fasterxml.jackson:jackson-bom: ${jackson2.version} [2.15.2] -> 2.17.0
Warning: Maven Central rate limit (429). Use -repository <mirror> or retry later.
org.sonatype.central:central-publishing-maven-plugin: Property missing: central-publishing-maven-plugin.version
Backup: v2 saved
Writing: 1 pom.xml

45 entries, 5 upgraded, 38 up to date, 2 skipped (Property missing: 1, Rate limited: 1)
```

mvnp caches metadata under `.mvnp/cache/metadata` and rate-limits requests (~300ms gap) to reduce load on Maven Central.

Use `-verbose` for detailed per-entry logs, or `-summary` for a full report.

**Examples**

```bash
mvnp upgrade -policy latest-releases ./my-project
mvnp upgrade -recursive --dry-run
mvnp upgrade -only org.apache.commons:commons-lang3
mvnp upgrade --select
mvnp upgrade --ignore-select
mvnp upgrade -include org.apache.commons:commons-lang3,org.junit:junit-jupiter
mvnp upgrade -ignore com.fasterxml.jackson:jackson-bom
mvnp upgrade -no-backup
```

### `list`

List upgradeable packages from `pom.xml`.

```bash
mvnp list [options] [directory]
```

| Option | Description |
|--------|-------------|
| `-numbered` | Show selection numbers |
| `-pom` | List a specific `pom.xml` |
| `-recursive` | Include child modules |
| `-include-parent` | Include parent version |

```bash
mvnp list --numbered
mvnp list -pom ./module-a/pom.xml
```

### `settings`

Manage shared configuration.

```bash
mvnp settings init [directory]     # create .mvnp/settings.json
mvnp settings init --global        # create ~/.config/mvnp/settings.json
mvnp settings show [directory]
```

Settings are loaded in this order:

1. Built-in defaults
2. Global settings: `~/.config/mvnp/settings.json`
3. Project settings: `.mvnp/settings.json`
4. Per-project overrides in global `projects` map
5. CLI flags (`-repository`, `-backup-dir`, `-no-backup`, `-include`, `-ignore`, ...)

Example `.mvnp/settings.json`:

```json
{
  "repository": "https://repo1.maven.org/maven2",
  "backupDir": ".mvnp/back",
  "autoBackup": true,
  "backupKeepCount": 2,
  "metadataCacheDir": ".mvnp/cache/metadata",
  "policy": "latest-releases",
  "ignore": [
    "com.fasterxml.jackson:jackson-bom",
    "tools.jackson:jackson-bom",
    "software.amazon.awssdk:bom"
  ]
}
```

See [settings.example.json](settings.example.json).

### `backup`

Create a versioned snapshot of `pom.xml` files.

```bash
mvnp backup [options] [directory]
```

| Option | Default | Description |
|--------|---------|-------------|
| `-recursive` | `false` | Include child module `pom.xml` files |
| `-backup-dir` | `.mvnp/back` | Backup storage directory |
| `-label` | | Optional label stored in manifest |
| `-list` | `false` | List existing backups |

**Backup layout**

```
.mvnp/back/
  v00001/
    manifest.json
    pom.xml
    module-a/pom.xml
  v00002/
    manifest.json
    pom.xml
```

Each backup version contains a `manifest.json` with metadata (version, timestamp, file list, label).

### `restore`

Restore `pom.xml` files from a backup.

```bash
mvnp restore [options] [directory]
```

| Option | Default | Description |
|--------|---------|-------------|
| `-version` | latest | Backup version to restore |
| `-backup-dir` | `.mvnp/back` | Backup storage directory |

**Examples**

```bash
mvnp restore
mvnp restore --version 2
mvnp restore -backup-dir /tmp/my-backups
```

## What Gets Upgraded

mvnp scans and upgrades explicit versions in:

- `<dependencies>`
- `<dependencyManagement>`
- `<plugins>`
- `<pluginManagement>`

Versions defined via `${property}` references are resolved from `<properties>` and upgraded there, while dependency/plugin version tags keep the property reference.

Example:

```xml
<properties>
  <jackson2.version>2.15.2</jackson2.version>
</properties>
<dependencyManagement>
  <dependencies>
    <dependency>
      <groupId>com.fasterxml.jackson</groupId>
      <artifactId>jackson-bom</artifactId>
      <version>${jackson2.version}</version>
    </dependency>
  </dependencies>
</dependencyManagement>
```

Running `mvnp upgrade` updates `<jackson2.version>` in properties, not the `${jackson2.version}` tag in the dependency block.

## Auto-backup Behavior

When running `mvnp upgrade` (without `--dry-run`):

1. mvnp evaluates available upgrades
2. If changes are needed, it automatically creates a new backup
3. Then it writes the upgraded `pom.xml` files

Use `"autoBackup": false` in settings or `--no-backup` on the command line to disable this behavior.

By default, only the latest **2** backup versions are kept (`"backupKeepCount": 2`). When a new backup is created, older versions are removed automatically. Set `"backupKeepCount": 0` to retain all backups.

## Development

```bash
go test ./...
go build -o mvnp .
```

## Roadmap

- `settings.xml` and private repository support
- Display available updates without upgrading
- Custom version rules (ignore alpha/beta patterns)
- Multi-module BOM import resolution

## License

MIT License. See [LICENSE](LICENSE) for details.

## Contributing

Contributions are welcome! Please open an issue or pull request.
