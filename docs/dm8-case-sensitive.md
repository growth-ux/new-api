# DM8 大小写敏感实例验证

当前适配按 `CASE_SENSITIVE=1`、独立 schema 验证。GORM 为模型对象生成带双引号的小写标识符；DM8 会将未加引号的标识符折叠为大写。因此，业务代码中的原始 `Where`、`Order`、聚合表达式和手写 SQL 必须使用方言引用的列名，或用 GORM `clause` 表达式生成标识符。不要对用户输入作为列名做字符串拼接。

## 本地隔离实例

验证使用单独的 DM8 容器、端口和数据卷，不复用已有 `dm8` 容器的数据。`CASE_SENSITIVE` 是初始化参数，改动已有数据卷的启动脚本不会重建数据库。当前本地验证镜像基于 `qinchz/dm8-arm64`，将其入口脚本中的 `CASE_SENSITIVE=0` 改为 `CASE_SENSITIVE=1` 后，以新数据卷初始化。启动后执行：

```sql
SELECT CASE_SENSITIVE();
```

返回值必须是 `1`。[init.sql](../database/dm8/init.sql) 中的对象名固定指定 `LDW_AI_AGG`，生产环境先创建该用户及同名 schema，再由部署账号执行 SQL；不能将原文件直接用于其他 schema。集成测试使用以 `NEWAPI_TEST_` 开头的隔离 schema，测试辅助仅在测试进程中把目标 schema 替换为该测试 schema。

部署账号需要创建表和索引的权限。应用运行账号按正常业务读写授予对象权限，不需要为启动迁移授予 `SELECT ANY DICTIONARY` 或建改表权限。驱动探针等明确要创建临时表的测试仍使用有相应权限的独立测试账号。本地旧测试账号 `NEWAPI_TEST_CASE1` 曾用于自动迁移验证，因此本次另建低权限账号做运行验证。

连接仍会检查 Ping 和当前 schema，启动不再预检 `SYS.SYSOBJECTS`。本地低权限验证确认运行账号没有 `SELECT ANY DICTIONARY`，无法创建表，仍可在预建表上完成启动和必要的业务读写。

## 回归验证

如果该测试 schema 正在供手动验证使用，可以运行以下不会清表的登录测试。它们会创建并清理自己的临时用户、验证记录和会话：

```sh
TEST_DM_DSN='dm://USER:PASSWORD@127.0.0.1:5237?schema=NEWAPI_TEST_CASE1' \
TEST_DM_SCHEMA=NEWAPI_TEST_CASE1 \
go test ./model -run '^TestDM8LoginVerificationProjection$' -count=1

TEST_DM_DSN='dm://USER:PASSWORD@127.0.0.1:5237?schema=NEWAPI_TEST_CASE1' \
TEST_DM_SCHEMA=NEWAPI_TEST_CASE1 \
go test ./controller -run '^TestDM8PasswordLoginHTTP$' -count=1
```

第二个测试覆盖密码登录、生成两步验证挑战、用 TOTP 完成登录及会话创建。

性能指标查询与累加写入可单独运行以下测试；测试在事务中执行并回滚写入：

```sh
TEST_DM_DSN='dm://USER:PASSWORD@127.0.0.1:5237?schema=NEWAPI_TEST_CASE1' \
TEST_DM_SCHEMA=NEWAPI_TEST_CASE1 \
go test ./model -run '^TestDM8PerfMetricsCaseSensitive$' -count=1
```

用量看板、异步任务、模型目录、审计、充值和签到的大小写敏感回归也可以在正在使用的测试 schema 上运行；测试使用事务回滚或只清理自己创建的记录：

```sh
TEST_DM_DSN='dm://USER:PASSWORD@127.0.0.1:5237?schema=NEWAPI_TEST_CASE1' \
TEST_DM_SCHEMA=NEWAPI_TEST_CASE1 \
go test ./model -run '^TestDM8(QuotaDashboard|AsyncTaskQueries|CatalogQueries|AuditQueries|LogAggregate|TopUpQueries|CheckinStats|AdminReadQueries)CaseSensitive$' -count=1
```

渠道能力判断、会话快照与账号绑定，以及兑换码状态筛选还可运行：

```sh
TEST_DM_DSN='dm://USER:PASSWORD@127.0.0.1:5237?schema=NEWAPI_TEST_CASE1' \
TEST_DM_SCHEMA=NEWAPI_TEST_CASE1 \
go test ./model -run '^TestDM8(ChannelSatisfyCaseSensitive|ChannelUpdateCaseSensitive|SessionSnapshotAndBindingCaseSensitive|AdminReadQueriesCaseSensitive)$' -count=1
```

这些回归覆盖了列出的查询族及部分写入路径；支付回调、所有管理操作和外部服务交互没有在本地逐一端到端运行。普通的 `go test ./...` 默认使用 SQLite，不能替代真实 DM8 验证。

密码中的特殊字符须按 URL 规则编码。下列测试会在**指定测试 schema 中执行建表、删表和写入**，不得指向业务 schema。

```sh
TEST_DM_DSN='dm://USER:PASSWORD@127.0.0.1:5237?schema=NEWAPI_TEST_CASE1' \
TEST_DM_SCHEMA=NEWAPI_TEST_CASE1 \
go test ./model -run '^TestDM8' -count=1
```

生产启动验证先在 `LDW_AI_AGG` 执行 `init.sql`，再以运行账号设置 `SQL_DSN` 启动应用。`TestDM8StartupWithInitSQL` 在隔离测试 schema 中替换限定名前缀并执行相同 SQL，连续进行两次启动数据初始化并确认没有结构变更。
