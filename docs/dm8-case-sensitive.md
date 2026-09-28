# DM8 大小写敏感实例验证

当前适配按 `CASE_SENSITIVE=1`、独立 schema 验证。GORM 为模型对象生成带双引号的小写标识符；DM8 会将未加引号的标识符折叠为大写。因此，业务代码中的原始 `Where`、`Order`、聚合表达式和手写 SQL 必须使用方言引用的列名，或用 GORM `clause` 表达式生成标识符。不要对用户输入作为列名做字符串拼接。

## 本地隔离实例

验证使用单独的 DM8 容器、端口和数据卷，不复用已有 `dm8` 容器的数据。`CASE_SENSITIVE` 是初始化参数，改动已有数据卷的启动脚本不会重建数据库。当前本地验证镜像基于 `qinchz/dm8-arm64`，将其入口脚本中的 `CASE_SENSITIVE=0` 改为 `CASE_SENSITIVE=1` 后，以新数据卷初始化。启动后执行：

```sql
SELECT CASE_SENSITIVE();
```

返回值必须是 `1`。使用专用测试用户和以 `NEWAPI_TEST_` 开头的 schema。连接用户需要创建/修改业务表的权限，还需要读取 DM8 元数据。当前 `gorm-dameng` 迁移器直接查询 `SYS.SYSOBJECTS` 等系统对象；本地验证用户使用了：

```sql
GRANT RESOURCE TO NEWAPI_TEST_CASE1;
GRANT CREATE SESSION TO NEWAPI_TEST_CASE1;
GRANT SELECT ANY DICTIONARY TO NEWAPI_TEST_CASE1;
```

生产环境可由 DBA 根据驱动实际访问的系统对象授予等效的最小权限。应用连接时会预检 `SYS.SYSOBJECTS`，缺少访问权限会返回明确错误，避免迁移器把元数据查询失败误判为表不存在。

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

启动验证可在相同独立 schema 上设置 `SQL_DSN` 后运行 `go run .`，检查迁移、定时任务和启动日志。现有 `TestDM8StartupMigration` 会清空测试 schema 的表，再执行首次迁移和重复迁移。
