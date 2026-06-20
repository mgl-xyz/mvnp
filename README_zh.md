# mvnp

用 Go 编写的 Maven 增强工具。**mvnp** 可安全地升级 `pom.xml` 中的依赖版本，并支持版本化备份与恢复。

[English](README.md)

## 功能特性

- **升级** `pom.xml` 中的依赖与插件版本
- **备份** `pom.xml` 文件，支持递增版本号
- **恢复** 任意历史备份（默认恢复最新一次）
- **自动备份**：每次升级前默认自动备份
- 升级策略与 [Maven Versions Plugin](https://www.mojohaus.org/versions/versions-maven-plugin/) 对齐
- 从 Maven Central 查询版本（支持自定义仓库地址）

## 安装

需要 Go 1.21 及以上版本。

```bash
git clone <repository-url>
cd mvnp
go build -o mvnp .
```

也可安装到 `PATH`：

```bash
go install .
```

## 快速开始

```bash
# 预览当前项目的可升级项
mvnp upgrade --dry-run

# 升级依赖（默认自动备份）
mvnp upgrade

# 手动创建备份
mvnp backup

# 列出所有备份
mvnp backup --list

# 恢复最新备份
mvnp restore

# 恢复指定版本备份
mvnp restore --version 1
```

## 命令说明

### `upgrade` — 升级依赖

升级当前目录或指定路径下的 `pom.xml` 依赖版本。

```bash
mvnp upgrade [选项] [目录]
```

**升级策略**

| 策略 | 说明 | 别名 |
|------|------|------|
| `latest-releases` | 升级到最新稳定 release 版本（默认） | `release`、`stable` |
| `latest-snapshots` | 将 SNAPSHOT 升级到最新 SNAPSHOT | `snapshot` |
| `latest-versions` | 升级到最新可用版本 | `latest` |
| `next-releases` | 升级到下一个 release 版本 | |
| `next-snapshots` | 升级到下一个 SNAPSHOT 版本 | |
| `next-versions` | 升级到下一个版本 | `next` |
| `releases` | 将 SNAPSHOT 替换为最新 release | `use-releases` |

**常用选项**

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `-policy` | `latest-releases` | 升级策略 |
| `-recursive` | `false` | 递归处理子模块 `pom.xml` |
| `-dry-run` | `false` | 仅预览，不写入文件 |
| `-no-backup` | `false` | 跳过升级前自动备份 |
| `-backup-dir` | `.mvnp/back` | 备份存储目录 |
| `-only` | | 仅升级指定 `groupId:artifactId` |
| `-include` | | 仅升级列表中的包 |
| `-ignore` | | 忽略列表中的包 |
| `-select` | `false` | 交互式选择要升级的包 |
| `-ignore-select` | `false` | 交互式选择要忽略的包 |
| `-pom` | | 指定 `pom.xml` 用于列表/选择 |
| `-include-parent` | `false` | 同时升级 parent 版本 |
| `-quiet` | `false` | 关闭进度输出 |
| `-verbose` | `false` | 显示详细多行进度 |
| `-summary` | `false` | 在 stdout 打印最终汇总报告 |

默认进度输出类似 `docker pull`，简洁单行展示：

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

mvnp 会在 `.mvnp/cache/metadata` 缓存元数据，并以约 300ms 间隔限速请求，减轻对 Maven Central 的压力。

需要详细日志用 `-verbose`，完整报告用 `-summary`。

**示例**

```bash
mvnp upgrade -policy latest-releases ./my-project
mvnp upgrade -recursive --dry-run
mvnp upgrade -only org.apache.commons:commons-lang3
mvnp upgrade --select
mvnp upgrade --ignore-select
mvnp upgrade -include org.apache.commons:commons-lang3
mvnp upgrade -ignore com.fasterxml.jackson:jackson-bom
mvnp upgrade -no-backup
```

### `list` — 列出包

列出 `pom.xml` 中可升级的包。

```bash
mvnp list [选项] [目录]
```

| 选项 | 说明 |
|------|------|
| `-numbered` | 显示编号，便于选择 |
| `-pom` | 指定某个 `pom.xml` |
| `-recursive` | 包含子模块 |
| `-include-parent` | 包含 parent 版本 |

```bash
mvnp list --numbered
mvnp list -pom ./module-a/pom.xml
```

### `settings` — 配置

管理公用配置。

```bash
mvnp settings init [目录]      # 创建 .mvnp/settings.json
mvnp settings init --global    # 创建 ~/.config/mvnp/settings.json
mvnp settings show [目录]
```

配置加载顺序：

1. 内置默认值
2. 全局配置：`~/.config/mvnp/settings.json`
3. 项目配置：`.mvnp/settings.json`
4. 全局配置里 `projects` 字段按项目路径覆盖
5. 命令行参数（`-repository`、`-backup-dir`、`-include`、`-ignore` 等）

示例 `.mvnp/settings.json`：

```json
{
  "repository": "https://repo1.maven.org/maven2",
  "backupDir": ".mvnp/back",
  "metadataCacheDir": ".mvnp/cache/metadata",
  "policy": "latest-releases",
  "ignore": [
    "com.fasterxml.jackson:jackson-bom",
    "tools.jackson:jackson-bom",
    "software.amazon.awssdk:bom"
  ]
}
```

参考 [settings.example.json](settings.example.json)。

### `backup` — 备份

创建 `pom.xml` 的版本化快照。

```bash
mvnp backup [选项] [目录]
```

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `-recursive` | `false` | 包含子模块 `pom.xml` |
| `-backup-dir` | `.mvnp/back` | 备份存储目录 |
| `-label` | | 可选标签，写入 manifest |
| `-list` | `false` | 列出已有备份 |

**备份目录结构**

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

每个备份版本包含 `manifest.json`，记录版本号、时间戳、文件列表和标签等信息。

### `restore` — 恢复

从备份恢复 `pom.xml` 文件。

```bash
mvnp restore [选项] [目录]
```

| 选项 | 默认值 | 说明 |
|------|--------|------|
| `-version` | 最新 | 要恢复的备份版本 |
| `-backup-dir` | `.mvnp/back` | 备份存储目录 |

**示例**

```bash
mvnp restore
mvnp restore --version 2
mvnp restore -backup-dir /tmp/my-backups
```

## 升级范围

mvnp 会扫描并升级以下区块中的显式版本：

- `<dependencies>`
- `<dependencyManagement>`
- `<plugins>`
- `<pluginManagement>`

通过 `${property}` 引用版本的依赖/插件，会从 `<properties>` 中解析实际版本并升级对应属性值，依赖/插件标签仍保留 `${property}` 引用。

示例：

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

执行 `mvnp upgrade` 会更新 properties 中的 `<jackson2.version>`，而不会修改 dependency 里的 `${jackson2.version}` 标签。

## 自动备份机制

执行 `mvnp upgrade`（未加 `--dry-run`）时：

1. 评估可升级项
2. 若有变更，自动创建新备份
3. 写入升级后的 `pom.xml`

使用 `--no-backup` 可关闭自动备份。

## 开发

```bash
go test ./...
go build -o mvnp .
```

## 路线图

- 支持 `settings.xml` 与私服仓库
- 仅查看可升级版本（不执行升级）
- 自定义版本规则（忽略 alpha/beta 等）
- 多模块 BOM import 解析

## 许可证

MIT License，详见 [LICENSE](LICENSE)。

## 参与贡献

欢迎提交 Issue 或 Pull Request！
