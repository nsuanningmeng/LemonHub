# Compose + MySQL 8.2：应用镜像更新前的只读预检

适用于已经运行的 Docker Compose 部署、容器内 MySQL 8.2、命名卷或绑定目录持久化。本流程只更新 LemonHub 应用；保持 MySQL 镜像、数据库连接、原数据卷以及 Compose 项目身份不变。仓库默认 Compose 使用 PostgreSQL，不能拿默认文件覆盖已有 MySQL 部署。

`scripts/mysql_compose_upgrade_preflight.py` 只调用 `docker compose config`、`docker inspect` 和 MySQL `SELECT`；不会停止、创建、替换容器，也不会写数据库。检查通过只说明列出的部署身份和保留条件一致，**不代表已备份、恢复演练通过、写入已排空，或对任何事故保证不丢数据**。

## 准备实际部署身份

需要 Python 3、Docker CLI、支持 `config --format json` 的 Docker Compose V2，以及执行当前 Docker 上下文检查的权限。务必使用原部署目录、原 Compose 文件顺序、项目名、环境文件和 Docker 上下文；不要连接到另一个 Docker daemon。脚本不会替换用户的 Docker 上下文。

下面命令只输出容器身份、镜像和挂载，不输出环境变量；将示例容器名替换为已有容器的名称或 ID：

```bash
docker inspect --type container original-app --format \
  'container={{.Id}} project={{index .Config.Labels "com.docker.compose.project"}} service={{index .Config.Labels "com.docker.compose.service"}} image={{.Image}}'
docker inspect --type container original-mysql --format \
  'container={{.Id}} project={{index .Config.Labels "com.docker.compose.project"}} service={{index .Config.Labels "com.docker.compose.service"}} image={{.Image}}'
docker inspect --type container original-mysql --format \
  '{{range .Mounts}}{{if eq .Destination "/var/lib/mysql"}}type={{.Type}} name={{.Name}} source={{.Source}} writable={{.RW}}{{end}}{{end}}'
```

从实际容器和已有部署记录核对原库名。`--expected-mysql-source` 对命名卷填写完整 `name`，对绑定目录填写绝对 `source`，不能填写一个新建目录或想要迁移到的新卷。应用 `/data` 不能代替 MySQL 的 `/var/lib/mysql`。

在原 Compose 中只准备应用目标镜像和停机宽限，保留全部既有环境及挂载。应用镜像应是验收过的明确发布版本或 digest，不要使用 `latest`；脚本不验证镜像内容或签名，digest 也不自动表示生产适用。`stop_grace_period` 必须大于应用 `SHUTDOWN_TIMEOUT_SECONDS`（默认 120 秒），例如 130 秒。不要改变 MySQL 镜像或数据库认证来同时完成另一项迁移。

## 执行预检

示例中所有 `original-*` 均需替换为核对过的原值。脚本路径使用当前代码仓库的绝对路径，但执行目录保持为原部署目录：

```bash
python3 /path/to/LemonHub/scripts/mysql_compose_upgrade_preflight.py \
  --file docker-compose.yml \
  --project-name original-project \
  --app-container original-app \
  --mysql-container original-mysql \
  --database original-database \
  --expected-mysql-source original-project_mysql_data
```

绑定目录改为例如 `--expected-mysql-source /srv/original/mysql_data`。使用多个 Compose 文件时按原顺序重复 `--file`；原来明确使用环境文件时添加 `--env-file /original/path/.env`。服务不叫 `new-api` / `mysql` 时分别设置 `--app-service` / `--mysql-service`。

脚本会拒绝以下情况：

- 容器不是指定 Compose 项目的原服务，网络或应用持久化挂载改变。
- 应用当前或目标配置存在自定义 `extra_hosts`、`dns`、`dns_search`、`dns_opt`、旧式容器 `links`，或应用把 MySQL 别名声明为自己的别名；这些设置可能把未变的连接串重定向到另一台服务器，需人工核查，不能仅按网络名一致放行。
- 原容器或目标配置缺少 `SQL_DSN`，或者主库、日志库、Redis、会话/验证码签名密钥配置发生变化。
- DSN 没有明确指向所选 MySQL 容器的 Docker 网络别名及内部 3306 端口，或原库名不一致。
- MySQL 数据没有持久化到 `/var/lib/mysql`，卷名/绑定路径不同、只读挂载，或有需人工核查的嵌套挂载。
- MySQL 镜像引用改变，应用镜像未固定，或目标设置会触发本地构建。
- 停机宽限不足，现有应用处于异常退出或 OOM 状态。
- 实际数据库不是 MySQL 8.2、实际数据目录不正确、主库没有既有用户或预期业务表、所选库为空。

成功输出只包含容器/镜像 ID、原挂载、库名、MySQL 版本、server UUID 和表数。Docker 解析配置及环境变量只在进程内存中比较，不写入报告；原始错误输出也不会回显。不要另外把 `docker compose config` 或完整 `docker inspect` 输出上传到工单，它们包含凭据。

MySQL 检查通过容器原有 `MYSQL_ROOT_PASSWORD` 或 `MYSQL_ROOT_PASSWORD_FILE` 读取本地认证信息，密码不进入命令行。使用其他认证方式、外部日志库、非标准数据目录、host 网络或无法识别的 DSN 时，脚本保守拒绝，需要按实际部署做人工等效检查。不要为了让脚本通过而临时改密码、换网络、创建空库或调整已有卷。

配置中的 `SESSION_SECRET` 未固定时，报告会显示 `session_secret_configured: false`；应用重启后旧登录态可能失效。已固定的密钥必须保留；不要在镜像切换时顺便轮换。

## 通过后仍必须完成的步骤

1. 入口停止接受新业务写入，排空所有应用实例的在途请求、流式连接、计费、退款及批量统计。旧版本没有的新退出保护不能追溯生效，退出码 0 也不能证明队列原本已排空。见[历史版本排空说明](mysql82-production-upgrade.md#先排空旧版本的写入)。MySQL 保持运行。
2. 使用大于旧进程退出预算的时限停止应用；核对退出码、OOM 状态与写入失败/待对账日志。不要忽略 137 或“提交结果不确定”的告警。
3. 对预检确认的**实际主库及独立日志库**做一致性备份，显式使用 `utf8mb4`，包含表结构、数据、触发器、事件和存储过程。备份前确认所有其他写入者及 DDL 操作已停止；仅 `--single-transaction` 不保护并发 DDL 或非事务表。不要把容器 `MYSQL_DATABASE` 初始环境值当成当前应用库名的唯一依据。
4. 备份放在数据库卷之外，以仅属主可读权限保存，并留有主机故障后可取回的副本。另存部署配置、应用持久化目录及必要的数据库账户/权限恢复资料。不要用复制运行中数据目录代替一致性备份。
5. 在独立 MySQL 8.2 实例和全新测试卷恢复，客户端同样指定 `utf8mb4`；核对字段值、余额、原始密钥、订单、订阅回执、日志及中文 emoji。非空文件、校验和、表数/行数相同均不足以证明完整恢复。备份中的 `--databases` 会选择原库名，恢复绝不能指向生产容器。
6. 对准备好的最终配置再次运行预检；全程保持同一目录、Compose 文件、项目名及环境文件。使用这些相同参数执行 **`pull <应用服务>` 和 `up -d --no-deps <应用服务>`**，不要省略服务名。若原部署有多个应用节点，先启动一个迁移主节点。
7. 检查启动迁移、原 MySQL 容器 ID/镜像 ID/挂载及 server UUID 不变，核对升级前后的关键业务记录。确认原库和业务正常后恢复流量。

不要运行 `down -v`、`docker volume rm/prune`、删除数据库宿主机目录或清空 Redis。若出现初始化页面，应先停应用核查错库/错卷，不要重新初始化。预检是一个时间点的检查，不会锁定 Docker 状态；检查后再改配置、换 Docker 上下文或执行其他部署会使结果失效。

需要回退时，保留升级后的库和现场，在独立新实例恢复升级前备份，用兼容的旧应用验证后再切换。已经恢复流量时，必须先保全并对账升级后的新增写入；直接恢复旧备份会丢掉新增数据。MySQL DDL 不能视为整个迁移的一个总事务，也不能假设旧应用可直接读取任意已迁移模式。

脚本回归测试：

```bash
python3 -m unittest discover -s scripts -p 'test_mysql_compose_upgrade_preflight.py' -v
```
