# MySQL 与容器升级的数据保留

## 结论与适用范围

本次 API 密钥分组修复不包含清库、删除密钥或自动重分组的数据库迁移。2026-09-23 在全新隔离的 MySQL 5.7.44、8.0.44 实例上，旧库迁移、重复启动迁移、历史空组密钥保留和数据目录重启检查均通过。升级后可继续使用原密钥值；空组密钥按[存量处理方案](api-key-group-remediation.md)由所属用户手动补选分组。

**只更新 LemonHub 应用容器，并继续连接原 MySQL 数据库，不会因为容器被替换而清空 MySQL 数据。** 这一结论要求 `SQL_DSN` 的数据库目标保持正确，MySQL 的 `/var/lib/mysql` 继续使用原持久化卷或原宿主机目录。它不保证磁盘故障、误删卷、指向新数据库或不受支持的 MySQL 大版本升级仍能保留数据。

本次没有连接生产环境，无法从仓库推断线上实际挂载。正式升级前必须在部署主机核对下述条件。

## 仓库检查

- `docker-compose.yml` 的 MySQL 示例使用 `mysql_data:/var/lib/mysql` 命名卷。启用 MySQL 服务时，也要启用文件底部的 `mysql_data` 卷声明；当前默认示例数据库是 PostgreSQL。
- 应用容器的 `./data:/data` 用于应用本地数据，不能替代 MySQL 的 `/var/lib/mysql` 持久化。
- `Dockerfile` 只打包应用，没有 MySQL 初始化或删除逻辑。启动时 `model/main.go` 执行增量迁移；未发现本次升级路径删除业务表的逻辑。
- 金额精度缩窄、充值状态缩窄、OAuth 身份冲突等迁移已有前置检查。异常数据会阻止启动，而不是静默截断。MySQL DDL 不能视为整个迁移的单一事务，迁移失败应保留现场并按日志处理，不能删库重建。
- 未配置 `SQL_DSN` 时应用会使用 SQLite；更换连接串、Compose 项目名或卷名可能让应用看到另一个空库。这需要恢复正确连接或挂载，不能把它当成需要重新初始化的正常升级。

## 已执行的隔离验证

测试基于 `a0600e2a89c649f871e62484f392fdf5594494fa` 加本次新增的升级回归测试执行。两个实例分别创建全新数据目录，仅监听本机回环地址，执行后干净关闭，未使用生产数据库或已有数据目录。

| 检查 | MySQL 5.7.44 | MySQL 8.0.44 |
| --- | --- | --- |
| 旧版完整业务库执行真实 `migrateDB()`，再执行第二次 | 通过 | 通过 |
| 用户、密钥、配额、支付记录、OAuth 身份、会话、任务和站点财务数据保留 | 通过 | 通过 |
| 密钥额度 INT → BIGINT 后，50 亿余额可读写且重复迁移不变 | 通过 | 通过 |
| 充值状态字段缩窄前拦截超长数据，正常记录与查询保留 | 通过 | 通过 |
| OAuth 身份与绑定按 MySQL 排序规则检测冲突 | 通过 | 通过 |
| 空组、空优先组、空白组、正常组密钥在两次启动迁移后保留原凭证、状态、分组与额度 | 通过 | 通过 |
| 干净停止 MySQL，使用相同 data dir 重启后保留全部哨兵字段与 50 亿余额 | 通过 | 通过 |

每个版本运行 6 个顶层集成测试，全部通过。新增测试是 `model/mysql_token_group_upgrade_test.go` 的 `TestMySQLTokenGroupUpgradePreservesExistingKeys`；仅接受名称为 `lemonhub_token_group_upgrade_test_*` 的全新空测试库，防止误用业务库。

复测命令如下，运行前应为每个测试设置其专属临时数据库 DSN，不得使用生产库：

```sh
go test ./model -run '^(TestMySQLRC25UpgradePreservesLegacyData|TestMySQLTokenQuotaMigrationPreservesData|TestMySQLTopUpMigrationSafety|TestPreflightExternalIdentityClaimsMySQLCollation|TestCustomOAuthBindingPreflightUsesClaimCollationMySQL|TestMySQLTokenGroupUpgradePreservesExistingKeys)$' -count=1 -v
```

本地 Docker Desktop 因 Inference socket 文件不可访问而无法启动；对已确认残留 socket 的清理被自动批准审查拒绝，因此本地没有完成真正的 Docker 容器重建实验，也没有更改 Docker 数据目录或卷。

`.github/workflows/ci.yml` 为 MySQL 5.7、8.0 增加容器重建检查：创建本次 CI 专属命名卷和容器，写入两条哨兵记录，保存完整表结构和字段值，停止并删除容器，使用同一命名卷创建新容器，再逐项核对表结构、数据和挂载卷名。资源名含工作流运行 ID、重试号与数据库版本；若同名资源已存在则拒绝复用，最终只清理本 job 创建的资源。**正式 Release 前必须确认两个版本的 CI 迁移与容器重建步骤全部成功。**

尚未实测生产数据量、线上存储驱动、断电恢复，或 MySQL 引擎跨大版本升级。这些检查不等同于备份恢复演练。

## 应用容器的升级步骤

下面命令用于 Linux/Bash，沿用现有 Compose 文件、目录和项目名。MySQL 服务名假设为 `mysql`，应用服务名为 `new-api`；外部 MySQL 应通过其既有备份与恢复流程处理。

1. 在部署主机确认 MySQL 的现有挂载，记录 `/var/lib/mysql` 对应的卷名或宿主机路径：

   ```sh
   docker inspect "$(docker compose ps -q mysql)" --format '{{json .Mounts}}'
   ```

   确认应用仍指向原主数据库；若使用独立日志数据库，也应备份该库。不要输出包含密码的完整连接串或将其放入工单。

2. 暂停应用写入并完成数据库备份；对异步计费/批量落库的部署，应先停接新请求并等待在途请求和待落库任务完成。以下示例适用于仓库 MySQL 服务的 `MYSQL_ROOT_PASSWORD` 与 `MYSQL_DATABASE` 配置：

   ```sh
   set -euo pipefail
   umask 077
   docker compose stop new-api
   backup="lemonhub-before-upgrade-$(date +%Y%m%d-%H%M%S).sql"
   docker compose exec -T mysql sh -c '
     export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"
     exec mysqldump -uroot --single-transaction --routines --events --triggers \
       --hex-blob --set-gtid-purged=OFF --databases "$MYSQL_DATABASE"
   ' > "$backup"
   test -s "$backup"
   ```

   校验备份命令成功，并在独立测试库验证恢复后再继续；仅有一个非空文件不代表备份可恢复。备份包含敏感数据，存放到受控且独立于数据库卷的位置。生产实例有额外参数或受限备份账户时，使用已验证的相应备份命令。

3. 将应用 `image` 固定到目标发布版本，保留数据库镜像版本、连接配置、卷名及 Compose 项目名，然后只更新应用：

   ```sh
   docker compose pull new-api
   docker compose up -d --no-deps new-api
   docker compose logs --tail=100 new-api
   ```

4. 确认迁移成功和应用健康，核对升级前后的用户、密钥、余额、充值订单以及关键记录。对历史空组密钥执行存量处理流程。若迁移报错，停止应用写入并先定位原因；需要回滚时，使用经过验证的旧应用版本及升级前备份，避免在已改动的数据库上盲目降级。

不要把 `docker compose down -v`、`docker volume rm`、`docker volume prune` 或删除宿主机数据库目录作为升级步骤。普通 `compose down` 默认不删除命名卷，但升级应用无需执行 `down`。Docker 官方确认卷内容独立于容器生命周期，并明确 `down --volumes` 会删除声明的命名卷：[卷生命周期](https://docs.docker.com/engine/storage/volumes/)、[Compose down](https://docs.docker.com/reference/cli/docker/compose/down/)。

若迁移部署目录或项目名，先确认实际旧卷，再显式引用它。可以在人工核对后使用 `external: true` 配合旧卷的完整名称，防止 Compose 自动创建另一个空卷；不能直接复制示例名称替换现有卷名。

## MySQL 引擎升级是独立操作

本次应用发布不要求更新 MySQL 镜像。仓库注释里的 `mysql:8.2` 只是部署示例，不是引擎升级指令；不得为了应用升级自动替换正在运行的 MySQL 版本。

若另行升级 MySQL，引擎兼容性、备份恢复和升级检查器需要单独验证。官方升级路径要求 5.7 先升到 8.0，再升到 8.4，不能跨过该中间系列。固定明确的目标版本，先在备份副本上演练，并按官方流程关闭和替换服务；不要把旧镜像直接重新挂到已被新版引擎升级过的数据目录上作为回滚方案。参见 [MySQL 升级路径](https://dev.mysql.com/doc/refman/8.4/en/upgrade-paths.html)与 [Docker 部署的数据持久化说明](https://dev.mysql.com/doc/refman/8.4/en/docker-mysql-more-topics.html)。
