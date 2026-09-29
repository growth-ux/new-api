# 达梦启动结构迁移分离实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans（建议在当前会话顺序实施）或经用户选择后使用 superpowers:subagent-driven-development。按下面的复选步骤执行。

**Goal:** 达梦先执行一个 `init.sql` 再启动应用，启动时不自动维护表结构，现有业务和其他数据库行为保持不变。

**Architecture:** 保留现有连接和启动流程，按主库、日志库各自的实际方言跳过达梦结构迁移。将夹在迁移函数中的用户数据回填抽成一个小函数，保留原有执行条件和顺序。数据库对象由唯一的 SQL 文件提前创建，不新增迁移框架。

**Tech Stack:** Go、现有 GORM 与 gorm-dameng、DM8、现有 testify 测试。不新增运行期依赖。

**Spec:** [已确认设计方案](../specs/2026-09-29-database-release-separation-design.md)。实施前先读该文件，本文不扩大其范围。

状态：已按计划实施。测试和辅助函数已落地，实际验收结果见本次交付说明。

## 全局约束

- 仅调整达梦启动路径。MySQL、PostgreSQL、SQLite、ClickHouse 等其他数据库保持现有初始化、自动迁移及相关行为。
- 只生成一个 `database/dm8/init.sql`；不新增日志脚本、README、校验脚本、生成器或部署命令。
- 不增加版本表、启动版本校验、迁移执行器、发布状态管理或自动结构检查。
- 历史数据回填、旧配置转换和正常业务初始化保持原逻辑；运行过程中的业务操作不改。
- 保留 `InitializeUserAuthVersions` → `InitializeExternalIdentityClaims` 的先后顺序、错误返回及主节点执行条件；`main.go` 中旧配置转换的调用保持原样。
- 不改邮箱业务行锁、CRUD 适配、权限系统、渠道修复、任务调度及正常密钥生成。
- 测试准备代码可以执行 SQL，应用启动代码不得读取或执行 `init.sql`。
- 只在隔离测试 schema 执行破坏性测试，沿用 `NEWAPI_TEST_` 前缀保护，不操作生产 `LDW_AI_AGG`。
- 保留已有 `AGENTS.md`、`CLAUDE.md` 和无关工作区变更；提交时逐文件选择，不使用 `git add .`。

## 审查重点

1. 跳过整段迁移后漏掉用户数据回填：任务 2 验证主节点回填和从节点不回填。
2. 共用主库的 `InitLogDB()` 直接调用审计迁移，绕过日志迁移函数：任务 2、3 单独覆盖。
3. 主库与日志库方言不同，错误地一起跳过：任务 2 的方言组合测试及任务 3 的实际启动验证覆盖。
4. SQL 漏建独立迁移的订阅表、达梦锁记录或正确软删除索引：任务 1 的完整性和业务约束测试覆盖。
5. 测试账号有字典或 DDL 权限，掩盖残留启动操作：任务 3 必须使用独立低权限账号验证，不能只看高权限测试通过。

## 文件范围

| 文件 | 调整 |
| --- | --- |
| `database/dm8/init.sql` | 唯一新增部署文件，包含全部结构、固定锁记录及使用注释 |
| `model/main.go` | 达梦结构迁移分支、额度字段检查分支、用户回填小函数和相应启动日志 |
| `model/dm8.go` | 删除迁移字典预检，保留连接、Ping、当前 schema 和错误处理 |
| `model/audit_log.go` | 审计表实际位于达梦时不自动建表 |
| `model/dm8_init_sql_test.go`（新增） | 正式 SQL 的测试装载、完整性和日志分段验证 |
| `model/dm8_startup_test.go`（新增） | 达梦启动分支、数据处理保留、低权限及混合方言验证 |
| `model/dm8_integration_test.go` | 使用正式 SQL 准备应用表，替换旧启动迁移测试 |
| `controller/dm8_login_integration_test.go` | 仅在必要时调整测试说明或前置条件，业务代码不改 |
| `docs/dm8-case-sensitive.md` | 修正现有文档中“生产启动需要字典权限、自动建表”的过时说明，不新增文档 |

其他生产文件原则上不改。原达梦迁移辅助函数可暂留给现有驱动测试使用，不为了本次任务做大范围死代码清理。

## Task 1：形成唯一初始化 SQL，并证明结构可用

**文件：** `database/dm8/init.sql`、`model/dm8_init_sql_test.go`、`model/dm8_integration_test.go`。

**接口：** 测试辅助 `applyDM8InitSQL(t *testing.T, db *gorm.DB, section string)`；`section` 仅接受 `all`、`log`。辅助只读取仓库 SQL，不生成结构，也不调用生产迁移。测试采用 `runtime.Caller` 定位到仓库文件，避免依赖 shell 当前目录。

- [x] **1. 写 SQL 验收测试并确认缺文件时失败。** 新增 `TestDM8InitSQLComplete` 和 `TestDM8InitSQLLogSection`。使用部署权限的隔离测试连接，分别在空 schema 执行全文或日志段。断言完整结构包含 `migrateDB()` 当前模型清单、`SubscriptionPlan`、`AuditLog`、`dm8TransactionLock` 的全部表、列、主键和声明索引；完整性依据放在测试中，不放进应用启动代码。
- [x] **2. 从模型和当前达梦适配整理 SQL。** 核对 `model/main.go` 的全模型列表、独立的订阅表、审计模型，以及 `dm8_dialector.go` 的字段映射。必要时在隔离参考库用旧迁移结果辅助导出，再人工核对；参考库与 SQL 验收库必须分开，避免残留对象掩盖漏建。无需提交生成工具。
- [x] **3. 编写 `init.sql`。** 使用当前模型对应的双引号小写标识符，正确保留 identity/自增、默认值、NULL 约束、索引、CLOB、时间类型、decimal 精度以及 BIGINT 额度。直接创建软删除 CASE 表达式唯一索引，不先建错误索引再修复；插入且仅插入一条 `normalized_email` 锁记录。权限、主题配置等原有业务初始化留在程序，不重复搬入 SQL。
- [x] **4. 在同一文件中明确日志分段。** 用独占行 `-- BEGIN LOG SCHEMA`、`-- END LOG SCHEMA` 包住 `logs`、`audit_logs` 及其索引，业务部分不得再次创建它们。全文用于主库兼日志库；独立达梦日志库执行该段。文件头说明仅用于空库、连接目标 schema、执行顺序及必要权限，不写固定账号密码、不创建用户、不授予宽泛权限、不添加自动删表。
- [x] **5. 完成测试装载和真实执行。** 脚本仅使用普通 DDL/DML，不包含客户端专属指令、匿名程序块或字符串内分号；每条语句用分号结束。测试辅助去掉整行 SQL 注释、按分号拆分并顺序 `Exec`，发现错误立即终止；这是针对该固定文件的测试辅助，不实现通用 SQL 执行器。日志段选择只识别上述两个标记。
- [x] **6. 将已有应用级 DM8 夹具改用 SQL。** 原先调用 `migrateDB()` 建表的测试，改为在明确受保护的隔离 schema 中清理、执行 `applyDM8InitSQL(..., "all")`。每个需要重建的测试独立准备，不依赖执行顺序；保留原本只读/只清理自己记录的测试性质，不使它们变成清库测试。独立驱动探针可继续为临时探针表使用 `AutoMigrate`。`TestDM8PrefillSoftDeleteUniqueness` 必须直接验证 SQL 创建的索引，不能再调用自动修复函数。
- [x] **7. 运行任务测试。** `TEST_DATABASE_MATRIX=1 go test ./model -run '^TestDM8(InitSQLComplete|InitSQLLogSection|PrefillSoftDeleteUniqueness)$' -count=1 -v`。预先配置 `TEST_DM_DSN`、`TEST_DM_SCHEMA`。预期全部 PASS；新库额度读写 `1<<40` 不丢失，锁记录计数为 `1`，活跃分组重名失败、软删除后重建成功，日志段不依赖任何业务表。不得将缺环境导致 SKIP 视为通过。
- [x] **8. 检查本任务差异并提交。** 只提交本任务 SQL 和测试改动，建议消息 `feat(dm8): add standalone initialization SQL`。

## Task 2：仅跳过达梦启动结构迁移，保留原数据处理

**文件：** `model/main.go`、`model/dm8.go`、`model/audit_log.go`、`model/dm8_startup_test.go`、`model/dm8_integration_test.go`。

**接口：** 保持 `InitDB() error`、`InitLogDB() error`、`migrateDB() error`、`migrateLOGDB() error`、`MigrateAuditLogs() error` 不变。只新增内部小函数 `initializeUserMigrationData() error`，按顺序调用现有两个用户回填函数，遇到错误立即返回。

- [x] **1. 写行为测试并验证旧代码不满足。** 新增 `TestDM8StartupSkipsSchemaWork`、`TestDM8StartupPreservesDataInitialization`、`TestStartupMigrationDialectRouting`。复用现有 `migrationSQLRecorder` 记录结构 SQL；按需要检查记录中的迁移元数据查询。测试准备阶段的建表和元数据读取不计入被测启动阶段。记录器用于可传入连接的迁移边界，真实启动另用低权限连接验证，不为测试新增生产配置或注入入口。

  必须断言：
  - SQL 建好后，达梦 `migrateDB()`、`migrateLOGDB()` 和直接 `MigrateAuditLogs()` 都没有结构变更或迁移元数据查询。
  - `checkDM8Compatibility` 仍进行连接和当前 schema 检查，但不访问 `SYS.SYSOBJECTS`；`ensureUserQuotaColumns(db, DatabaseTypeDM)` 不查询字段定义。
  - 在合法测试表中准备 `auth_version=0` 及未回填的 Telegram 绑定：主节点初始化后认证值为 `1`、身份归属补齐，重复执行结果一致；真实 `InitDB()` 从节点路径不执行这些回填。不能把两个回填函数都跳过后误当测试通过。
  - 用迁移边界的组合测试覆盖 `DM/DM`、`DM/SQLite`、`SQLite/DM`、`SQLite/SQLite`；达梦跳过，非达梦仍执行结构迁移。内存 SQLite 只用于快速验证分支及数据处理，不能代替达梦语法和权限验收。

- [x] **2. 做最小生产改动。**
  - `ensureUserQuotaColumns` 的提前返回条件增加达梦，不改变其他方言；该函数在主从判断前调用，不能仅让 master 跳过。
  - 从 `checkDM8Compatibility` 删除 `catalogRows` 的系统字典读取及对应错误信息；其余连接检查保留。
  - `migrateDB()` 在任何结构检查/修复之前判断达梦，达梦仅执行 `initializeUserMigrationData()`。将原非达梦路径中的两个回填调用替换为同一小函数，位置和错误行为不变；不把订阅表迁移提前到回填之前。原达梦锁表/索引初始化不再进入生产启动路径。
  - `migrateLOGDB()` 按日志方言在达梦时直接返回；`MigrateAuditLogs()` 同样按日志方言跳过达梦，覆盖 `InitLogDB()` 无独立日志 DSN 的直接调用。
  - 保留 `InitDB()`、`InitLogDB()` 的 master/slave 条件，不改 `main.go` 的旧配置转换和其他业务初始化。不新增关闭迁移的环境变量。
  - 达梦启动不再打印误导性的“开始数据库迁移”日志；其他数据库原有日志和流程保持。

- [x] **3. 替换旧启动迁移测试。** 将 `TestDM8StartupMigration` 改为 `TestDM8StartupWithInitSQL`：使用正式 SQL 准备结构，连续执行启动初始化两次并检查没有结构操作，固定锁记录仍只有一条。不能继续用 `AutoMigrate` 给这个测试兜底建库。
- [x] **4. 运行测试。** 快速回归：`go test ./model -run '^(TestStartupMigrationDialectRouting|TestMigrationSchemaStability|TestMigrateRetiredFrontendOptions.*|TestInitializeExternalIdentityClaims.*|TestClickHouse.*)$' -count=1 -v`。达梦验证：`TEST_DATABASE_MATRIX=1 go test ./model -run '^TestDM8Startup' -count=1 -v`。预期实际执行的用例全部 PASS，数据回填和非达梦分支行为不变；显式列出未配置的外部数据库测试。
- [x] **5. 检查本任务差异并提交。** 确认未修改运行期行锁、角色初始化、任务调度和 CRUD 适配，建议消息 `refactor(dm8): separate startup schema work from data initialization`。

## Task 3：低权限启动和业务回归，修正现有说明

**文件：** `model/dm8_startup_test.go`、必要的现有 DM8 测试前置条件、`docs/dm8-case-sensitive.md`。本任务不增加产品功能或部署文件。

**接口：** 测试继续使用 `TEST_DM_DSN`、`TEST_DM_SCHEMA`。低权限连接仅在测试中通过 `TEST_DM_RUNTIME_DSN` 提供；独立达梦日志测试使用 `TEST_DM_RUNTIME_LOG_DSN`。这些不是新增应用配置，生产仍使用现有 `SQL_DSN`、`LOG_SQL_DSN`。新测试缺少所需连接时，普通单元测试允许跳过；显式设置 `TEST_DATABASE_MATRIX=1` 时必须报错，不能静默跳过。

- [x] **1. 准备隔离环境。** 先查看用户指定的本地达梦容器及现有测试实例，确认实际参数和可用 schema；当前集成测试要求 `CASE_SENSITIVE=1`。不重建或改动含现有数据的实例参数，不修改生产账号权限。由部署身份在专用 `NEWAPI_TEST_` schema 执行正式 SQL，给独立运行身份业务对象所需权限，保持其无字典查询、建改表权限及继承高权限角色；日志 schema 只执行同文件的日志段。
- [x] **2. 新增并运行 `TestDM8StartupWithRuntimeAccount`。** 以运行身份的真实连接调用 `InitDB()`、`InitLogDB()`，覆盖 master/slave、共库、独立达梦日志和达梦/SQLite 混用，运行后关闭连接并恢复全局状态。测试不得并行修改 `DB`、`LOG_DB` 和数据库类型全局变量；真正的多实例验证用独立进程。
- [x] **3. 验证权限确实受限。** 在测试准备的独立检查阶段确认运行身份读取 `SYS.SYSOBJECTS` 失败、创建独立探针表失败，同时在预建业务表中必要 CRUD 成功。权限探测不加入应用代码、不计入启动 SQL 统计；若探针意外成功，则权限验收失败，由部署身份清理测试对象。
- [x] **4. 验证缺结构时不自动修复。** 在独立空 schema 上调用达梦启动路径，允许正常业务查询因缺表失败，不要求所有节点都立即失败；用部署身份检查没有创建任何应用表。日志表缺失时同样不得自动创建。不增加专门的空库检测功能。
- [x] **5. 运行现有业务回归，并补 `TestDM8EmailUniquenessWithInitSQL`。** 验证邮箱并发写入仅一个成功、正常返回邮箱占用错误；验证软删除名称复用、登录/两步验证、会话、渠道选择、额度扣减、订阅、日志和审计。只新增/调整测试，不修改这些业务实现。旧配置转换和默认主题初始化继续按原逻辑执行。
- [x] **6. 验证真实多实例启动。** SQL 完成初始化后，以低权限账号启动两个独立应用进程、使用不同端口和测试配置。通过启动日志和业务探测确认均能就绪，无迁移权限错误、无重复建表；测试结束停止自己启动的进程。现有数据回填仍可执行，不将 DML 误认为结构迁移。若发现原有并发数据初始化问题，记录具体证据并向用户说明，不顺带改业务。
- [x] **7. 完成回归并记录范围。** 配置好隔离连接后依次运行 `TEST_DATABASE_MATRIX=1 go test ./model -run '^TestDM8' -count=1 -v`、`go test ./controller -run '^TestDM8PasswordLoginHTTP$' -count=1 -v`。共用 schema 的包不能同时执行破坏性夹具；先跑模型包，再确保 SQL 结构完整后跑控制器。普通回归在清除外部数据库测试连接和 `TEST_DATABASE_MATRIX` 的独立子进程中执行 `go test ./model ./controller ./service/authz -count=1`，避免再次触发共享库清理。另运行任务 2 的非达梦矩阵，MySQL/PostgreSQL 有测试实例时使用现有 `TEST_MYSQL_DSN`、`TEST_POSTGRES_DSN`；无实例必须报告未验证，不能宣称实库通过。ClickHouse 现有 SQL/TTL 测试应通过，相关生产逻辑不得改动。
- [x] **8. 修正文档并交付。** 更新现有 `docs/dm8-case-sensitive.md` 中的生产迁移权限要求和旧测试说明，明确高权限仅用于测试建库/驱动探针，应用按先 SQL 后启动运行。执行说明集中在 `init.sql` 注释；最终汇报代码变化、唯一 SQL 文件、实际测试结果和未验证项，不新增报告文件。执行 `git diff --check`，只提交本任务文件，建议消息 `test(dm8): verify startup with precreated schema and restricted account`。

## 完成标准

- 新增的 SQL 部署文件只有 `database/dm8/init.sql`，可在空达梦库中独立建立所需结构。
- 达梦启动不建改表、不修索引、不做迁移字典检查，用户回填和原有业务逻辑保留。
- 主库与日志库独立按方言处理，其他数据库原有自动迁移不受影响。
- 实际 SQL、低权限启动及业务回归有明确结果；未跑的环境明确说明。
- 不含数据库版本管理、自动升级、额外部署工具或未经用户确认的业务改动。
