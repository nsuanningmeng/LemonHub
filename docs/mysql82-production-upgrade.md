# MySQL 8.2 部署：仅更新 LemonHub 应用

适用于应用 v0.4.45、MySQL `mysql:8.2`，且数据库使用 `${APP_PATH}/mysql_data:/var/lib/mysql` 宿主机绑定目录的部署。本文不要求更新 MySQL 或 Redis 镜像。应用 `/data` 挂载不是 MySQL 数据目录的替代品。

## 升级前固定部署身份

在现有 Compose 目录、使用原项目名和原环境文件操作。记录当前应用镜像 ID、MySQL 镜像 ID，以及数据库实际挂载源：

```bash
docker inspect "$(docker compose ps -q new-api)" --format '{{.Image}}'
docker inspect "$(docker compose ps -q mysql)" --format '{{.Image}} {{json .Mounts}}'
```

核对 `/var/lib/mysql` 的 `Source` 确实是正在使用的绝对路径；同时核对应用 `/data` 和 `/app/logs` 的旧路径。不要为了通过检查而创建一个新的空 `mysql_data` 目录。移动 Compose 文件、改变 `APP_PATH`、项目名或 `.env` 都可能使应用连接另一个空库。若出现初始化页面，先停止应用并检查连接和挂载，不要重新初始化。

保留数据库地址、库名 `new-api`、独立日志库（如有）、Redis 连接和命名空间、会话密钥、节点名及 `Asia/Shanghai` 时区。不要把完整连接串、密钥或备份上传到公开 issue。

## 先排空旧版本的写入

生产配置启用了 `BATCH_UPDATE_ENABLED=true`。部分用量计数和数据看板统计会短暂留在进程内存；用户钱包和令牌的预扣并不因此全部改为内存写入。

先在入口停止接受新业务请求，保留旧应用运行，等待已有请求、流式连接、后台计费任务、退款以及最后一轮统计写入完成。若有其他实例连接同一数据库，也必须停止它们产生新写入。检查落库错误、待对账回执和提交结果不确定的告警；固定等待若干秒或单条“刷新完成”日志，不能证明仍有生产者的队列已经为空。

**首次停机运行的仍然是 v0.4.45。新版本的退出修复和超时设置不能追溯保护旧进程。** 如不能确认旧版本已经完成所有写入，暂停切换并保留现场，不能把正常健康检查当作排空证明。

本次隔离演练已复现：v0.4.45 在下一轮批处理前正常退出时，钱包和令牌已完成结算，但 `users.used_quota`、`users.request_count`、`channels.used_quota` 仍未落库。停止旧应用前必须核对这些批量统计的写入；仅有请求结束、钱包余额正确或更长的停止宽限都不足以证明它们已保存。

给新版本设置外层停机宽限，至少大于应用内部退出预算，例如在现有服务配置中增加：

```yaml
services:
  new-api:
    stop_grace_period: 130s
    environment:
      # 保留现有全部环境项；在原列表中增加这一行。
      - SHUTDOWN_TIMEOUT_SECONDS=120
```

这段仅展示需要追加的配置，不应替换原有完整 `environment` 列表。对尚未重新创建的旧容器，Compose 文件里的新设置并未生效；完成旧版本排空后，使用显式停止时限：

```bash
docker compose stop -t 130 new-api
docker inspect "$(docker compose ps -a -q new-api)" \
  --format 'exit={{.State.ExitCode}} oom={{.State.OOMKilled}}'
```

保留应用退出日志。退出码 137、OOM、强制终止、写入失败或待对账告警都需要先处理；退出码 0 本身也不替代旧版本的排空检查。MySQL 和 Redis 保持运行。

## 备份并验证恢复

在全部写入停止后导出原数据库，显式使用 utf8mb4。以下命令从 MySQL 容器现有环境读取凭据，不把密码写入命令行；使用独立备份位置，并检查每条命令的退出状态：

```bash
set -euo pipefail
umask 077
backup="lemonhub-before-upgrade-$(date +%Y%m%d-%H%M%S).sql"
docker compose exec -T mysql sh -c '
  export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"
  exec mysqldump --default-character-set=utf8mb4 -uroot \
    --single-transaction --routines --events --triggers \
    --hex-blob --set-gtid-purged=OFF --databases "$MYSQL_DATABASE"
' > "$backup"
test -s "$backup"
sha256sum "$backup"
```

若使用独立日志数据库，另行备份该库；保存应用持久目录及部署配置的受控副本。备份应位于数据库目录之外，并有主机故障时仍能取回的副本。不要复制运行中的 MySQL 数据目录来代替一致性备份。

在另一个 MySQL 8.2 实例和新测试卷恢复，恢复客户端同样指定 `--default-character-set=utf8mb4`。备份含 `--databases`，会选择原库名，因此必须确认目标容器是独立恢复实例，绝不能向生产容器执行恢复。恢复后核对表结构、用户、原始密钥、余额、订单、订阅回执、日志、NULL/0 差异和中文 emoji。非空 SQL 文件、SHA256 或“导入成功”单项都不是完整恢复验证。

## 只替换应用

把应用 `VERSION` 固定为已经验收的发布标签或 digest，记录旧镜像 ID。保持 MySQL 镜像和现有容器不变，然后仅操作应用服务：

```bash
docker compose pull new-api
docker compose up -d --no-deps new-api
docker compose logs --tail=200 new-api
```

先由一个迁移主节点启动，核对启动迁移成功、原库连接、关键字段和健康状态，再启动其他实例并恢复业务流量。MySQL 的多条 DDL 不是一个总事务；预检或迁移失败时保留数据库和日志，不要清库重建。

新版本会拒绝无法正确读取或解析的持久化设置。若历史设置不合法导致启动停止，按日志定位对应设置并在备份后修正；不要通过删除全部设置、重置密钥或重新初始化来绕过检查。

不要使用不带服务名的 `compose pull/up` 来顺便更新 `mysql:8.2` 或 `redis:latest`。不要使用 `down -v`、删除宿主机数据库目录、清理数据库卷或刷新 Redis 作为应用升级步骤。

## 回退和适用边界

需要回退时先停止新写入，保留升级后的库，再按验证过的备份在独立新实例恢复，并使用兼容的旧应用版本核对后切换连接。已恢复流量时，必须先核算升级后新增的数据；直接恢复旧备份会丢掉这些新写入。不要在已由新引擎升级的数据目录上运行旧 MySQL 镜像。

代码审查和隔离测试能够验证明确版本及故障场景，不能替代生产主机的实际挂载、备份恢复及排空检查。内存队列与普通日志不是 WAL；断电、SIGKILL、磁盘损坏、持续数据库故障或提交结果不确定时，不能承诺任意数据均可自动恢复。遇到不确定提交应对账，不能把未知金额盲目重放。

提供的生产配置还使用了空密码的 MySQL root 连接。建议在独立维护中配置受控密码及满足迁移需求的专用账户，并同步验证连接；不要在这次应用镜像切换中单方面更改数据库认证，造成启动失败。
