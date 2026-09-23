# DM8 最小改动接入 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在保留上游结构和现有三库行为的前提下，为 new-api 增加经真实数据库和目标 ARM 环境验证的 DM8 主库、共库日志及独立日志支持。

**Architecture:** 继续使用现有 model、GORM、DB/LOG_DB 和唯一一份生产 AutoMigrate 模型清单。DM 专用方言、迁移、写入和锁适配放入新增文件；原文件只保留必要的连接入口及兼容分支。先验证驱动，再接入应用，避免先改大量业务代码后才发现驱动不满足要求。

**Tech Stack:** Go 1.25.1；GORM v1.25.12；SQLite、MySQL、PostgreSQL；DM8/ARM；testify；目标平台优先验证 CGO_ENABLED=0。

**Spec:** [已批准的设计](../specs/2026-09-21-dm8-minimal-integration-design.md)。用户在本会话中以“继续吧 写计划”批准进入计划编写。

**Status:** 已按“有真实 DM8 实例”的路径实施核心接入（任务 1—6 及任务 4/5 的必要运行时分支），并对真实 DM8（linux/arm64 容器）完成连接、生产迁移幂等、服务启动/重启、注册/登录/会话/token/渠道搜索等验证；详见文末“执行证据”。任务 7—8 的完整四库矩阵、发布版升级夹具、物理 ARM 主机运行与官方驱动评估尚未完成。

## Global Constraints

- 当前项目使用 Go 1.25.1、GORM v1.25.12、MySQL 方言 v1.5.7、PostgreSQL 方言 v1.5.9。首选保留这组版本，新增 DM 驱动；不以接入 DM 为理由同时升级 ORM。
- 保持 SQLite、MySQL、PostgreSQL 的现有行为，保持 ClickHouse 仅用于独立日志库的限制。
- 所有受影响数据库行为必须验证真实 SQLite、MySQL >= 5.7.8、PostgreSQL >= 9.6 及目标 DM8；用到最低版本相关行为时补测最低版本。
- 已确认目标为 DM8、ARM 架构。完整数据库构建号、服务器操作系统、ARM 位数、兼容模式、大小写设置和测试实例可用性尚未提供。
- 推荐初始验证配置使用 UTF-8、大小写敏感（CASE_SENSITIVE=1）的实例，优先评估 MySQL 兼容模式（COMPATIBLE_MODE=4）以减少现有语法差异。
- 不对整段 SQL 做 ToUpper、反引号全局替换或正则重写，不改变参数、字符串字面值及用户提供的数据。
- 继续执行现有 migrateDB()/migrateLOGDB() 和唯一一份 AutoMigrate 模型清单。
- 测试只操作隔离的测试数据库/schema，显式恢复全局 DB 和类型状态，相关用例不并行污染全局变量。
- 测试连接池由夹具持有并关闭；每次业务子测试开始前清理自己拥有的夹具数据，结束时检查清理错误。既有 truncateTables 只注册结束清理，不能当作前置清空。用固定 ID 的用例必须从已验证的空表开始。
- 发布验证必须检查矩阵用例实际运行，缺 DSN 导致 Skip 不能计为通过。
- 跨数据库历史数据搬迁、零停机切换、双写、自动回切不属于本次适配。
- 保留全部 new-api / QuantumNous 项目标识。没有故障证据不改 controller、service、前端、relaykit 或现有 CI 结构。
- 涉及额度、充值、退款测试前完整阅读 `.agents/rules/billing.md`；保持金额/额度计算和账务不变量。
- 涉及认证持久化、锁或错误脱敏前阅读设计引用的 OWASP 认证与会话管理指南；使用 ASVS 5.0.0 的已识别条目进行回归，不把原有代码等同于安全证明。
- 新增或实质改写的 Go 测试使用 require/assert，优先复用现有行为测试；本功能默认只新增一个集中测试文件 `model/dm8_integration_test.go`。
- Go JSON 编解码走 common 包；驱动适配文件不引入独立业务模型和价格规则。

## Review Focus

1. DSN 中含 `@:/?#%` 的凭证、主库/日志使用不同方言：连接身份不能误解析，日志不能泄漏凭证或受到另一连接的回调污染。任务 2、7 覆盖。
2. 自增主键未提供、非主键唯一键冲突、两种唯一键并存：正确区分插入、忽略和更新，RowsAffected 与回填 ID 属于本次操作。任务 3 覆盖。
3. 超过 64 KiB 的中文/emoji 文本、显式 column 标签、别名及 NULL：存储和读取无截断、不清空、不改变大小写或空值语义。任务 2、4 覆盖。
4. 不同连接竞争同一邮箱/认证断言、锁等待取消、回滚后重试：不得双重占用，不能泄漏锁或消耗未提交的凭证。任务 5 覆盖。
5. 元数据权限不足、索引唯一性识别错误、迁移中途退出：不能把错误当成对象不存在，不能丢原数据，恢复后重复启动不反复 DDL。任务 6、8 覆盖。

## 文件与接口地图

以下路径均相对仓库根目录。代码基线为 `6e9de44a7`，设计提交为 `d5ce9a043`；执行前重新定位函数，行号仅作为导航。

| 文件 | 任务 | 边界 |
| --- | --- | --- |
| `model/dm8.go`（新增） | 2 | 连接、服务器配置检查、DM 错误识别 |
| `model/dm8_dialector.go`（新增） | 2 | DM 专用结构化标识符和类型映射 |
| `model/dm8_migrator.go`（新增） | 2、6 | 元数据映射、索引/约束、DM 升级与锁表 |
| `model/dm8_create.go`（新增） | 3 | 仅在候选驱动存在已证实缺口时增加冲突写入适配 |
| `model/dm8_compat.go`（新增） | 4、5 | DM 时间及邮箱串行化 |
| `model/dm8_integration_test.go`（新增） | 2—8 | 同一个测试文件中组织契约、业务和迁移回归 |
| `common/database.go`、`model/main.go` | 2、4、6 | 类型、连接入口、引用、位宽、迁移钩子 |
| `model/gorm_logger.go`、既有 logger 测试 | 2 | DM 服务端错误脱敏 |
| `model/channel.go`、`topup.go`、`subscription.go`、`redemption.go` | 4 | 显式字段引用短分支 |
| `model/db_time.go`、`usedata_rankings.go`、`user.go` | 4、5 | 时间、取整、邮箱锁 |
| `model/option_primary_key_migration.go` | 6 | DM 专用恢复路径入口，保留其他方言实现 |
| `model/migration_dialector_test.go` | 6 | 扩展现有迁移矩阵 |
| `.env.example`、`go.mod`、`go.sum` | 2、7 | 驱动固定版本、现有配置的 DM 示例 |
| `scripts/test-dm8-matrix.sh`（新增） | 8 | 发布前严格验证，缺实例直接失败 |

任务依赖为 `1 → 2 → 3 → 4 → 5 → 6 → 7 → 8`。任务 2 的探针只使用专用夹具，不调用完整 InitDB；任务 6 才接入完整迁移，避免尚未补齐写入与锁时误认为应用已经可运行。

各任务提交前，按该任务 Files 清单逐个暂存实际改动文件，运行 `git diff --cached --check` 与 `git diff --cached --stat`；不使用 `git add .`，不包含已有的用户改动。代码步骤中的接口契约以任务 1 的实测驱动结果为前提，禁止将注释形式的方法清单粘贴为未实现的生产代码。

### Task 1: 固定环境与驱动，验证可用的扩展边界

**Files:**
- Read: `go.mod`、`Dockerfile`、`model/main.go`、`model/auth_flow.go:179`、`model/migration_dialector.go`。
- Create during execution: 系统临时目录中的独立 probe 模块，不将试验依赖加入根 go.mod。
- Update after execution: 本计划末尾的“执行证据”记录。

**Interfaces:**
- Consumes: 操作系统/ARM 位数、DM8 构建号、隔离测试 schema、`TEST_DM_DSN`、官方 Go/GORM V2 包或其可重现获取地址。
- Produces: 一个固定来源的 `gorm.Dialector` 工厂、驱动错误类型/错误码接口、元数据大小写和 CLOB 策略、经过实测的服务器配置。工厂在任务 2 统一封装为 `openDM8(dsn string) (*gorm.DB, error)`，不向业务层暴露供应商 API。

- [ ] **Step 1: 记录目标环境，只输出非秘密信息。** 在目标应用主机运行下列命令；数据库主机不同则分别记录。通过授权的 DM 客户端查询版本与当前 schema，并记录 CASE_SENSITIVE、COMPATIBLE_MODE、字符集和时区的实际值。客户端命令/系统视图必须使用目标版本文档确认，不能凭记忆修改实例配置。

```sh
uname -s
uname -m
go version
go env GOOS GOARCH CGO_ENABLED
git rev-parse HEAD
```

```sql
SELECT BANNER FROM V$VERSION;
SELECT SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA') FROM DUAL;
```

若实际目标不是 64 位 ARM，先核对当前项目的整数位宽和构建约束；本文的 linux/arm64 命令不能用于证明该目标可运行。无实例时该任务保持未完成，不用 SQLite 替代 DM 探针。

- [ ] **Step 2: 检查官方包，再建立可重现的社区对照探针。** 记录官方包校验和、许可、依赖树和 `Open/New` 的实际签名；不使用本机绝对路径 replace 作为交付方式。社区对照仅使用以下固定版本，其已知风险需要被测试复现，不能直接认定合格：

```sh
DM8_PROBE_DIR=$(mktemp -d "${TMPDIR:-/tmp}/new-api-dm8-probe.XXXXXX")
cd "$DM8_PROBE_DIR"
go mod init newapi-dm8-probe
go get github.com/godoes/gorm-dameng@v0.6.0
go list -m all
```

- [ ] **Step 3: 建立最小连接/元数据探针。** 下列测试完整保存为临时模块的 `probe_test.go`。变量由安全环境注入，要求 TEST_DM_SCHEMA 为预先分配的 `NEWAPI_TEST_` 前缀 schema；不输出 DSN。

```go
package probe

import (
    "os"
    "strings"
    "testing"

    dameng "github.com/godoes/gorm-dameng"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
)

func TestDriverConnection(t *testing.T) {
    dsn := os.Getenv("TEST_DM_DSN")
    expected := os.Getenv("TEST_DM_SCHEMA")
    if dsn == "" || !strings.HasPrefix(expected, "NEWAPI_TEST_") {
        t.Fatal("configure TEST_DM_DSN and an isolated TEST_DM_SCHEMA")
    }
    db, err := gorm.Open(dameng.Open(dsn), &gorm.Config{
        PrepareStmt: false,
        Logger: logger.Default.LogMode(logger.Silent),
    })
    if err != nil {
        t.Fatal("DM connection failed; inspect sanitized driver diagnostics")
    }
    pool, err := db.DB()
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { _ = pool.Close() })
    var actual string
    if err := db.Raw("SELECT SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA') FROM DUAL").Scan(&actual).Error; err != nil {
        t.Fatal("unable to verify current schema")
    }
    if actual != expected {
        t.Fatal("current schema differs from isolated test schema")
    }
}
```

临时模块作为非产品探针使用 Go 标准 testing；进入仓库的新增测试统一改用 require/assert。官方候选以相同测试输入替换唯一的工厂调用，不改变预期。

- [ ] **Step 4: 验证驱动关键契约。** 将任务 2—3 的记录夹具、唯一键/RowsAffected 断言及任务 6 的索引断言移植到临时模块；先记录原驱动的实际失败。只在临时模块补最小扩展，证明结构化标识符、CLOB、元数据及冲突回调能够修复，不先改根模块。

```sh
GOWORK=off go test -count=1 -v ./...
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOWORK=off go test -c -o dm8-probe-linux-arm64.test
```

在已确认的 Linux ARM64 主机运行该探针并连接真实 DM8；其他目标使用已确认的 GOOS/GOARCH。编译通过与目标运行通过分别记录。

- [ ] **Step 5: 完成选型门槛。** 进入任务 2 必须有：固定包/校验和与导入路径；不提升现有 GORM 的依赖树；实际 ARM 运行；已验证的标识符/元数据策略；冲突处理缺口及已通过的最小修复。任何一项不满足都不进入生产接入。该门槛允许否决候选，而不允许把“能连接”记为通过。

此任务的产物是实测结论，无法事先杜撰官方包版本或生产工厂代码。选定后把工厂的准确代码与来源写入执行证据；后续任务消费这一结果。若必须升级 GORM 或采用不同的标识符设计，先修订本计划和 spec，再进入后续任务。

### Task 2: 接入 DM 连接、类型和标识符，建立隔离测试夹具

**Files:** Create `model/dm8.go`、`model/dm8_dialector.go`、`model/dm8_migrator.go`、`model/dm8_integration_test.go`；Modify `common/database.go:5`、`model/main.go:140`、`model/gorm_logger.go:69`、`model/gorm_logger_test.go`、`go.mod`、`go.sum`。

**Interfaces:**
- Consumes: 任务 1 固定的 `gorm.Dialector` 和真实配置。
- Produces: `openDM8(dsn string) (*gorm.DB, error)`；`checkDM8Compatibility(db *gorm.DB) error`；`sanitizeDM8Error(err error) (error, bool)`；`dm8Dialector` 实现 `gorm.Dialector` 与 `gorm.SavePointerDialectorInterface`。
- Test fixtures: `openDM8TestDB(t *testing.T) *gorm.DB` 只连接并检查隔离 schema；`useDatabaseTestState(t *testing.T, db *gorm.DB, kind common.DatabaseType)` 保存/恢复全局状态；`prepareDM8ContractRecords(t *testing.T, db *gorm.DB)` 只建立/清理下述专用表。

- [ ] **Step 1: 写真实往返测试。** 创建以下夹具，并在 `TestDM8IdentifiersAndValues` 中执行。新增文件导入 testing、strings、os、fmt、errors、time、context、common、require/assert、gorm/schema/clause 等实际用到的包，完成后 gofmt 删除未用项。

```go
type DM8ContractRecord struct {
    ID       int64   `gorm:"primaryKey"`
    Key      string  `gorm:"column:key;size:128;uniqueIndex:idx_dm8_contract_key"`
    Group    string  `gorm:"column:group;size:64"`
    Ref      *string `gorm:"size:128;uniqueIndex:idx_dm8_contract_ref"`
    Body     string  `gorm:"type:text"`
    Meta     JSONValue `gorm:"type:json"`
    Enabled  bool
    Quantity int64
}

func (DM8ContractRecord) TableName() string { return "dm8_contract_records" }

func TestDM8IdentifiersAndValues(t *testing.T) {
    db := openDM8TestDB(t)
    prepareDM8ContractRecords(t, db)
    input := DM8ContractRecord{
        Key: "MiXeD-Key", Group: "default",
        Body: strings.Repeat("中文🙂", 10000),
        Meta: JSONValue(`{"enabled":false,"count":0}`),
        Quantity: 1 << 40,
    }
    require.NoError(t, db.Create(&input).Error)
    require.Positive(t, input.ID)
    var got DM8ContractRecord
    require.NoError(t, db.Where("id = ?", input.ID).First(&got).Error)
    assert.Equal(t, input.Key, got.Key)
    assert.Equal(t, input.Body, got.Body)
    assert.JSONEq(t, string(input.Meta), string(got.Meta))
    assert.Equal(t, input.Quantity, got.Quantity)
    assert.False(t, got.Enabled)
    assert.Nil(t, got.Ref)
    var count int64
    require.NoError(t, db.Model(&DM8ContractRecord{}).
        Where(clause.Eq{Column: clause.Column{Name: "key"}, Value: "mixed-key"}).Count(&count).Error)
    assert.Zero(t, count)
}
```

同一测试文件追加 `TestDM8IdentifierAliases`：使用 `db.Table("dm8_contract_records AS r").Select("r.id AS record_id")` 读入 `struct{ RecordID int64 }` 及 map，明确结果列名策略；追加 NULL、空串、空 JSON 数组及精度为 10,6 的 decimal 读回断言。大字段长度服务于真实截断边界，不用作性能测试。

- [ ] **Step 2: 实现隔离夹具并先运行红色测试。** `openDM8TestDB` 在 TEST_DM_DSN 缺失时：普通开发运行 Skip；`TEST_DATABASE_MATRIX=1` 时 Fatal。连接后校验当前 schema 精确等于 TEST_DM_SCHEMA 且有 NEWAPI_TEST_ 前缀，校验完成前不执行 DDL/DML。`prepareDM8ContractRecords` 只清理这张专用表，并注册最终清理。

`useDatabaseTestState` 保存 DB、LOG_DB、主/日志类型、RedisEnabled、BatchUpdateEnabled、LogConsumeEnabled，在 cleanup 中恢复并调用 initCol；以 `DB=LOG_DB=db`、Redis=false、Batch=false、LogConsume=true 开始。不要调用 t.Parallel，也不要修改原 TestMain。

```sh
TEST_DATABASE_MATRIX=1 GOWORK=off go test ./model -run '^TestDM8(IdentifiersAndValues|IdentifierAliases)$' -count=1 -v
```

预期首次在连接入口/类型适配缺失或往返断言处失败。缺环境属于门槛失败，不算回归已被证明。

- [ ] **Step 3: 增加最小连接入口。** 使用任务 1 的固定工厂实现 openDM8：先解析并检查 dm scheme，创建带现有 logger、PrepareStmt=false 的配置，打开连接，调用 checkDM8Compatibility，失败关闭刚建立的池并返回脱敏错误。不要日志输出原始 DSN；连接成功后沿用 InitDB/InitLogDB 原连接池设置。

```go
// common/database.go 的常量组中新增。
DatabaseTypeDM DatabaseType = "dm"

// chooseDB() 中，放在原 MySQL 兜底分支之前。
if strings.HasPrefix(dsn, "dm://") {
    db, err := openDM8(dsn)
    return db, common.DatabaseTypeDM, err
}
```

在 `sanitizeDBError` 加入下述入口；sanitizeDM8Error 通过任务 1 验证的具体错误类型和数值码识别服务端错误，保留安全的错误码，不依赖错误文本包含某单词。保持 errors.Is 对上下文取消/超时等控制错误有效。

```go
if sanitized, ok := sanitizeDM8Error(err); ok {
    return sanitized
}
```

在既有 logger 测试中加入真实 DM 唯一冲突与 wrapped error；通过现有日志器写入 bytes.Buffer，断言不包含测试凭证 `MiXeD-Key` 和 DSN 密码。

- [ ] **Step 4: 移入经过验证的 Dialector/Migrator 扩展。** 实现下列方法边界；保留实际驱动的 BindVarTo、Explain、DefaultValueOf，显式转发 SavePoint/RollbackTo，不能仅嵌入接口而丢失可选能力。

```go
var _ gorm.Dialector = (*dm8Dialector)(nil)
var _ gorm.SavePointerDialectorInterface = (*dm8Dialector)(nil)

// 方法必须在本任务中提供完整实现。
// Name() string -> "dm"
// QuoteTo(clause.Writer, string) -> 仅规范化结构化标识符并引用
// DataTypeOf(*schema.Field) string -> DM 专用类型映射
// Migrator(*gorm.DB) gorm.Migrator -> 引用同一包装方言的迁移器
// SavePoint(*gorm.DB, string) error / RollbackTo(*gorm.DB, string) error
```

规范化规则：物理表/列/索引采用大写；Go 模型与 DBName 逻辑映射保持原样；显式 column 标签、别名和元数据参数应用同一规则。元数据返回名称还原为模型可匹配的逻辑名称。引用 SQL 中的字符串与参数原样保留。

类型规则：text/json/长文本映射经过探针验证的 CLOB；有索引的 string 按声明长度保留可索引类型；无界且不参与索引的配置/密钥字段避免有限默认 VARCHAR 截断；保持整数位宽和 decimal 精度。检查自定义 GormDBDataType 绕过 DataTypeOf 的情况，在 FullDataTypeOf 做同样映射，并测试嵌入迁移器实际使用了该映射。对不支持的类型明确报错，禁止静默改变精度。

- [ ] **Step 5: 补 DSN 和隔离回归。** `TestDM8ConnectionFailure` 使用关闭的测试端口，断言 error 非空、db 不可用、没有产生本地 SQLite 文件；`TestDM8ConnectionEscaping` 用 net/url 构造特殊字符密码的专用测试账号连接；`TestDM8DialectorIsolation` 先后创建 DM 与 SQLite，确认 SQLite 冲突写入/命名仍保持原结果。特殊字符账号无实例配置时登记未完成，不能用字符串解析单测冒充连接验证。

```sh
TEST_DATABASE_MATRIX=1 GOWORK=off go test ./model -run '^TestDM8(IdentifiersAndValues|IdentifierAliases|ConnectionFailure|ConnectionEscaping|DialectorIsolation)$' -count=1 -v
GOWORK=off go test ./model -run '^Test(SanitizeDBError|GormLogger|JSONColumn)' -count=1
go list -m gorm.io/gorm gorm.io/driver/mysql gorm.io/driver/postgres
git diff --check
```

- [ ] **Step 6: 提交本任务。** 检查实际 diff 只包含本任务文件，执行 `git commit -m "feat(db): add isolated DM8 connection and dialect support"`。根模块三种已有方言的实测矩阵在任务 8 完成前，不能标记整个适配已完成。

### Task 3: 保证冲突写入、RowsAffected 和主键回填

**Files:** Modify `model/dm8_dialector.go`、`model/dm8_integration_test.go`；Create `model/dm8_create.go`（只有任务 1 证明驱动缺口时）；不修改业务模型的唯一键定义。

**Interfaces:** Consumes `openDM8`、`DM8ContractRecord` 与任务 2 的测试夹具；Produces `registerDM8Create(db *gorm.DB) error`，只替换当前 DM 连接的 create 回调，普通已验证路径继续调用原 create 回调。

- [ ] **Step 1: 写非主键冲突的红色测试。**

```go
func TestDM8ConflictContract(t *testing.T) {
    db := openDM8TestDB(t)
    prepareDM8ContractRecords(t, db)
    first := DM8ContractRecord{Key: "same-key", Body: "first"}
    result := db.Create(&first)
    require.NoError(t, result.Error)
    assert.EqualValues(t, 1, result.RowsAffected)
    duplicate := DM8ContractRecord{Key: first.Key, Body: "ignored"}
    result = db.Clauses(clause.OnConflict{
        Columns: []clause.Column{{Name: "key"}}, DoNothing: true,
    }).Create(&duplicate)
    require.NoError(t, result.Error)
    assert.Zero(t, result.RowsAffected)
    var stored DM8ContractRecord
    require.NoError(t, db.First(&stored, first.ID).Error)
    assert.Equal(t, "first", stored.Body)
    updated := DM8ContractRecord{Key: first.Key, Body: "updated"}
    result = db.Clauses(clause.OnConflict{
        Columns: []clause.Column{{Name: "key"}},
        DoUpdates: clause.AssignmentColumns([]string{"body"}),
    }).Create(&updated)
    require.NoError(t, result.Error)
    require.NoError(t, db.First(&stored, first.ID).Error)
    assert.Equal(t, "updated", stored.Body)
    assert.Equal(t, first.ID, stored.ID)
}
```

- [ ] **Step 2: 列出并测试当前实际用法。** 运行 rg 定位生产 OnConflict；追加 `TestDM8ConflictMultipleUniqueKeys`、`TestDM8BatchIdentity`、`TestDM8ConflictRollback`。其中 Ref 与 Key 分别唯一：指定 Key 为冲突目标时，撞 Ref 必须报错；无目标 DoNothing 按其契约忽略唯一冲突；非唯一错误必须返回。批量测试输入三条固定 Key，查回每个 Key 对应的 ID 并逐个核对回填值；冲突后下一次插入不能借用别人的 ID。

```sh
rg -n 'OnConflict|AssignmentColumns|DoUpdates|UpdateAll' model --glob '*.go' --glob '!**/*_test.go'
TEST_DATABASE_MATRIX=1 GOWORK=off go test ./model -run '^TestDM8(Conflict|BatchIdentity)' -count=1 -v
```

- [ ] **Step 3: 只在 DM 回调内修复。** 以任务 1 实测有效的原子写入为基础实现冲突列选择：显式 Columns 优先；未指定时保留全部真实唯一约束语义；不能总按主键匹配。MERGE 或捕获唯一冲突的具体路线必须保证事务内可继续执行、目标约束错误能区分，以及批量行为明确。若驱动不能提供可靠的单条 ID，使用同连接的受支持返回机制；必要时将需回填 ID 的批量插入拆成同一事务中的逐条插入，不能假设 ID 必然连续。

注册范围由以下代码限定，原回调作为局部闭包捕获；复杂实现保留在具备独立契约的 DM 写入方法中。

```go
func registerDM8Create(db *gorm.DB) error {
    if db.Dialector.Name() != "dm" {
        return fmt.Errorf("DM create adapter received %s", db.Dialector.Name())
    }
    original := db.Callback().Create().Get("gorm:create")
    if original == nil {
        return errors.New("DM create callback is missing")
    }
    return db.Callback().Create().Replace("gorm:create", func(tx *gorm.DB) {
        if _, present := tx.Statement.Clauses["ON CONFLICT"]; !present {
            original(tx)
            return
        }
        createDM8WithConflict(tx)
    })
}
```

`createDM8WithConflict(tx *gorm.DB)` 在本任务定义，消费 `clause.OnConflict` 与 `callbacks.ConvertToCreateValues` 生成的值，使用同一 Statement.ConnPool/Context 执行；写回本次真实 RowsAffected 与主键。保留 Before/After 钩子；普通批量主键测试若也发现原驱动问题，同样纳入此回调的受控分支。

对每种现有子句建立输入/目标约束/结果记录。尚未实现的现有用法属于任务失败；未来未知子句返回明确不支持错误，不吞错、不回退到无锁的先查后写。

- [ ] **Step 4: 回归安全相关调用。** 在只迁移 AuthFlow/ExternalIdentityClaim 的夹具中调用现有 `ClaimExternalAuthAssertion` 两次，第二次必须为 ErrAuthFlowConsumed；两用户抢同一外部身份只能保留一个归属。并发测试使用两条实际连接，收集各自结果后由主测试线程 assert。

- [ ] **Step 5: 测试通过后提交。**

```sh
TEST_DATABASE_MATRIX=1 GOWORK=off go test ./model -run '^TestDM8(Conflict|BatchIdentity|AssertionClaim|IdentityClaim)' -count=1 -v
git diff --check
git commit -m "fix(db): preserve DM8 conflict and identity semantics"
```

### Task 4: 修正必要的 SQL 分支与数据库时间

**Files:** Modify `model/main.go:43`、`channel.go:396`、`topup.go:150`、`subscription.go:573`、`redemption.go:147`、`db_time.go:7`、`usedata_rankings.go:51`；Create/Modify `model/dm8_compat.go`；Test `model/dm8_integration_test.go`。其他查询只有实测失败才扩展。

**Interfaces:** Consumes `DatabaseTypeDM`、完整 DM 方言；Produces `dm8DatabaseTimestamp(db *gorm.DB) (int64, error)`；其余函数签名不变。

- [ ] **Step 1: 通过行为测试覆盖原始片段。** 在 `TestDM8Queries` 中建立固定两条 Channel、三条 Redemption、两笔 TopUp、一笔 SubscriptionOrder，调用现有搜索与订单方法，核对筛选、顺序、分页和写入状态。复用现有 `TestSearchRedemptionsFiltersAndPaginates` 的数据与期望，通过子测试调用而不复制业务实现。

针对统计增加以下独立用例，不按生成的 SQL 字符串断言：

```go
func TestDM8RankingBuckets(t *testing.T) {
    db := openDM8TestDB(t)
    useDatabaseTestState(t, db, common.DatabaseTypeDM)
    require.NoError(t, db.Migrator().DropTable(&QuotaData{}))
    require.NoError(t, db.AutoMigrate(&QuotaData{}))
    t.Cleanup(func() { assert.NoError(t, db.Migrator().DropTable(&QuotaData{})) })
    rows := []QuotaData{
        {ModelName: "model-a", CreatedAt: 3599, TokenUsed: 7},
        {ModelName: "model-a", CreatedAt: 3600, TokenUsed: 11},
        {ModelName: "model-a", CreatedAt: 3601, TokenUsed: 13},
    }
    require.NoError(t, db.Create(&rows).Error)
    got, err := GetRankingQuotaBuckets(1, 7200, 3600)
    require.NoError(t, err)
    assert.Equal(t, []RankingQuotaBucket{
        {ModelName: "model-a", Bucket: 0, Tokens: 7},
        {ModelName: "model-a", Bucket: 3600, Tokens: 24},
    }, got)
}
```

- [ ] **Step 2: 运行红色测试并按失败点补分支。** initCol() 将 DM 引用与布尔值分别设置，主/日志类型各自判断，保留原两种路径：

```go
if common.UsingMainDatabase(common.DatabaseTypeDM) {
    commonGroupCol = `"GROUP"`
    commonKeyCol = `"KEY"`
    commonTrueVal = "1"
    commonFalseVal = "0"
}
if common.UsingLogDatabase(common.DatabaseTypeDM) {
    logGroupCol = `"GROUP"`
    logKeyCol = `"KEY"`
}
```

已有 models/base_url/trade_no/key 的显式引用，在原分支旁增加对应大写 DM 引用。例如：

```go
if common.UsingMainDatabase(common.DatabaseTypeDM) {
    refCol = `"TRADE_NO"`
}
```

分桶用经实测的 FLOOR 表达式，为 DM 增加条件；不重写其他数据库表达式：

```go
if common.UsingMainDatabase(common.DatabaseTypeMySQL) ||
    common.UsingMainDatabase(common.DatabaseTypeDM) {
    return fmt.Sprintf("FLOOR(created_at / %d) * %d", bucketSize, bucketSize)
}
```

- [ ] **Step 3: 明确 DM 时间路径并测试错误可见性。** 在已锁定的 MySQL 兼容配置中，先实测 `SELECT UNIX_TIMESTAMP()` 返回 UTC Unix 秒，再用于 dm8DatabaseTimestamp；如果目标不支持，任务 1 的部署配置门槛尚未满足，不能靠应用时间回退通过测试。

```go
func dm8DatabaseTimestamp(db *gorm.DB) (int64, error) {
    var ts int64
    if err := db.Raw("SELECT UNIX_TIMESTAMP()").Scan(&ts).Error; err != nil {
        return 0, err
    }
    if ts <= 0 {
        return 0, errors.New("DM returned an invalid database timestamp")
    }
    return ts, nil
}
```

GetDBTimestamp 的 switch 增加 DM case 调用此函数，保留已有统一 fallback。`TestDM8DatabaseTime` 直接断言该函数成功，并以数据库端固定时间换算及受控时区验证单位/时区；关闭连接后直接调用必须返回 error，不能由 fallback 遮蔽。

- [ ] **Step 4: 验证 LIKE、通配符转义与 TRUNCATE。** 用参数 `a_b`、`a%b`、`a!b`、引号和中文建立精确输入/输出；日志测试包含 literal 与通配符两种模式。ability 重建前插入一条记录，调用既有重建入口后校验最终集合；如果 TRUNCATE 与事务契约冲突，只给 DM 分支使用原事务内可回滚的删除。

- [ ] **Step 5: 通过回归并提交。**

```sh
TEST_DATABASE_MATRIX=1 GOWORK=off go test ./model -run '^TestDM8(Queries|RankingBuckets|DatabaseTime|LogFilters|AbilityRebuild)$' -count=1 -v
GOWORK=off go test ./model -run '^Test(SearchRedemptions|GetFlowQuotaData|RechargeEpay)' -count=1
git diff --check
git commit -m "fix(db): adapt DM8 query branches without restructuring models"
```

### Task 5: 邮箱互斥、认证重放、账务与任务并发

**Files:** Modify `model/dm8_compat.go`、`model/dm8_migrator.go`、`model/user.go:392`、`model/dm8_integration_test.go`；Read `.agents/rules/billing.md`、`model/locking.go`、现有 auth/session/quota/payment/system-task 测试。

**Interfaces:** Produces `dm8TransactionLock`、`ensureDM8TransactionLocks(db *gorm.DB) error`、`lockDM8NormalizedEmail(tx *gorm.DB) error`。Consume `lockForUpdate`，不改变公开业务 API。

- [ ] **Step 1: 写数据库锁测试。** 定义固定锁记录，在独立事务中获取一次；第二条连接使用 NOWAIT 探针获取同一行，必须返回锁冲突。第一个事务回滚后同一探针成功。NOWAIT 仅用于验证真实锁，不添加到生产流程；使用目标版本验证的语法，不以 sleep/运行时长证明锁存在。

```go
type dm8TransactionLock struct {
    Name string `gorm:"primaryKey;size:64"`
}

func (dm8TransactionLock) TableName() string { return "dm8_transaction_locks" }

func lockDM8NormalizedEmail(tx *gorm.DB) error {
    var anchor dm8TransactionLock
    return lockForUpdate(tx).
        Where("name = ?", "normalized_email").
        First(&anchor).Error
}
```

ensureDM8TransactionLocks 在迁移主节点创建表与记录。创建时遇到唯一冲突仅在验证该固定记录确实存在后视为幂等；其他错误返回。业务路径禁止临时创建缺失记录。

- [ ] **Step 2: 添加最小业务分支。** 在 lockNormalizedEmail 已有空邮箱提前返回之后的 switch 中加入：

```go
case common.UsingMainDatabase(common.DatabaseTypeDM):
    return lockDM8NormalizedEmail(tx)
```

此分支在原事务内执行，所有非空邮箱共用一个数据库锁。审阅所有调用方的锁顺序，确保先邮箱锚点再用户行；相反顺序若存在必须以最小局部修改统一，避免死锁。

- [ ] **Step 3: 建立明确的并发邮箱结果测试。** `TestDM8ConcurrentEmail` 使用两条连接和同一个开始 channel，两个事务依次调用 lockNormalizedEmail、ensureEmailAvailableWithTx，再 Create 不同 Username 但相同规范化邮箱。结果只能一成功、一 ErrEmailAlreadyTaken，最终该邮箱行数为 1；空邮箱两用户均可写。锁表行缺失时必须返回错误且无用户写入。等待中的事务取消后，下一事务仍能获得锁。

单次事务代码使用已有 API，所有 require/assert 在测试主 goroutine 检查结果：

```go
err := db.Transaction(func(tx *gorm.DB) error {
    if err := lockNormalizedEmail(tx, " Taken@Example.COM "); err != nil {
        return err
    }
    if err := ensureEmailAvailableWithTx(tx, "taken@example.com", 0); err != nil {
        return err
    }
    return tx.Create(&User{
        Username: username, Email: "taken@example.com", AffCode: username,
        Password: "unused-test-hash", AuthVersion: 1,
    }).Error
})
```

username 由两个固定输入 `dm8-email-one`、`dm8-email-two` 提供；不要将整个插入函数替换为探针 SQL，既有公开 Insert/BindEmail 入口还需各自调用一次验证接线。

- [ ] **Step 4: 复用安全与业务契约。** 定义 `TestDM8BusinessContracts`，在完整准备好所需表、Redis/cache/设置状态后使用下列子测试表。此处列举的函数均已经存在，不复制各层夹具；每个子测试 cleanup 执行完再运行下一个。

```go
cases := []struct {
    name string
    run func(*testing.T)
}{
    {"assertion_once", TestExternalAuthAssertionCanOnlyBeClaimedOnce},
    {"flow_expiry", TestAuthFlowExpiryIsEnforced},
    {"flow_rollback", TestConsumeAuthFlowWithActionRollsBackTogether},
    {"identity_owner", TestExternalIdentityClaimEnforcesSingleOwnerAtomically},
    {"session_revoke", TestUserSessionCreateListAndRevokeOne},
    {"session_rotation", TestRotateUserSessionRefreshRaceAndReuse},
    {"password_reset_sessions", TestPasswordResetBumpsAuthVersionAndRevokesSessions},
    {"quota_reservation", TestTryReserveQuotaWithoutRedis},
    {"payment_once", TestRechargeEpayCreditsQuotaExactlyOnce},
    {"payment_provider", TestCompleteSubscriptionOrder_RejectsMismatchedPaymentProvider},
    {"task_claim", TestSystemTaskLockPreventsConcurrentClaim},
    {"task_identical_update", TestUpdateSystemTaskStateIdenticalPayloadDoesNotLoseLock},
}
for _, tc := range cases {
    t.Run(tc.name, tc.run)
}
```

上述业务套件的完整启动接线在任务 6 完成后执行；本任务先用所需表的测试夹具验证锁与认证入口。不得把需 Redis 的现有测试未经初始化直接放到 DM 上，也不能修改测试预期来掩盖驱动差异。

- [ ] **Step 5: 增加两项实际竞争用例。** 余额为 100 的同一用户，两连接各预留 60，只能一次成功，最终余额 40；同一待支付订单两连接回调，最终余额只增加一次。使用真实事务与唯一约束，阻塞/错误均保留并检查；不对整个业务事务盲目重试，避免重复外部副作用。

- [ ] **Step 6: 运行并提交。**

```sh
TEST_DATABASE_MATRIX=1 GOWORK=off go test ./model -run '^TestDM8(EmailLock|ConcurrentEmail|ConcurrentQuota|ConcurrentPayment|AssertionClaim|IdentityClaim)$' -count=1 -v
git diff --check
git commit -m "fix(db): preserve DM8 transactional and authentication guarantees"
```

### Task 6: 复用生产迁移清单，保证约束与中断恢复

**Files:** Modify `model/dm8_migrator.go`、`model/main.go:279`/`:320`/`:395`、`model/option_primary_key_migration.go:21`、`model/migration_dialector_test.go`、`model/dm8_integration_test.go`。

**Interfaces:** Produces `prepareDM8Schema(db *gorm.DB, isLog bool) error`、`verifyDM8Schema(db *gorm.DB, isLog bool) error`、`migrateDM8OptionPrimaryKey(db *gorm.DB) error`。Consuming driver metadata methods and `ensureDM8TransactionLocks`；不另建生产模型清单。

- [ ] **Step 1: 扩展既有迁移矩阵，写索引契约测试。** 在 TestMigrationSchemaStability 加入 dm，使用 TEST_DM_DSN 并按任务 2 校验隔离 schema。新增 TestDM8IndexMetadata：创建一个普通索引和一个唯一索引，GetIndexes 的 Unique 必须分别 false/true，列顺序正确；再尝试重复插入确认数据库确实执行唯一约束。

```go
indexes, err := db.Migrator().GetIndexes(&DM8ContractRecord{})
require.NoError(t, err)
found := false
for _, index := range indexes {
    if strings.EqualFold(index.Name(), "idx_dm8_contract_key") {
        unique, known := index.Unique()
        require.True(t, known)
        assert.True(t, unique)
        assert.Equal(t, []string{"key"}, index.Columns())
        found = true
    }
}
assert.True(t, found)
```

- [ ] **Step 2: 明确迁移前后顺序。** 在现有 migrateDB 前部调用 prepareDM8Schema(DB,false)，成功后继续原模型清单；所有原迁移结束后调用 verifyDM8Schema(DB,false)。migrateLOGDB 使用 isLog=true；LOG_DB=DB 路径也要覆盖日志约束验证。以下为主库接线形式：

```go
if common.UsingMainDatabase(common.DatabaseTypeDM) {
    if err := prepareDM8Schema(DB, false); err != nil {
        return err
    }
}
```

prepareDM8Schema 恢复未完成 options 修复、检查已有表的必要类型，初始化 DM 锁表；独立日志路径仅执行日志相关检查。verifyDM8Schema 读实际约束、锁锚点和位宽，有错误返回启动失败。不要依赖 HasTable/HasIndex 的 bool 接口传播元数据查询错误，关键预检使用能返回 error 的查询并区分“不存在”和“没有权限”。

- [ ] **Step 3: 保留钱包位宽与 decimal 检查。** is64BitIntegerType 增加 DM 经真实 ColumnTypes 确认的类型名称，仅认可实际 64 位整数，不接受任意 NUMERIC 或以字符串包含 int 判断。TestDM8QuotaSchema 在 BIGINT 夹具保存 1<<40 并读回；INT 夹具调用 ensureUserQuotaColumns 必须报错，不能设置 SKIP_64BIT_QUOTA_SCHEMA_CHECK。

- [ ] **Step 4: 验证软删除唯一性。** TestDM8PrefillUniqueness 创建同名未删除记录必须失败；软删除后可创建同名新记录；恢复旧记录造成两条活跃记录时必须失败。两次 AutoMigrate 后结果不变。若 DM 不支持部分索引，由 DM Migrator 对该索引建立并识别经目标版本实测的等价条件表达式索引，其他方言的标签不变。

候选等价表达式如下，必须用上述实际数据验证 NULL 和恢复行为后采用：

```sql
CREATE UNIQUE INDEX "UK_PREFILL_NAME" ON "PREFILL_GROUPS"
    (CASE WHEN "DELETED_AT" IS NULL THEN "NAME" ELSE NULL END);
```

- [ ] **Step 5: 为 options 修复增加 DM 入口和可恢复状态机。** 不运行默认双重 rename 后无条件删除临时表的清理逻辑。已有主键正常时立即返回；冲突值无法确定时失败，保留全部数据。复制完成后核对记录与唯一性，再交换名字，保留原表作为备份。

```go
// migrateOptionPrimaryKey 的 DB 非 nil 检查之后。
if db.Dialector.Name() == "dm" {
    return migrateDM8OptionPrimaryKey(db)
}
```

使用固定 DM 专用临时表 `dm8_options_rebuild` 和备份表 `dm8_options_backup`。每一步只依赖可观察的表状态，操作前校验源/临时内容一致；禁止删除未经验证的原始数据：

| options | rebuild | backup | 动作 |
| --- | --- | --- | --- |
| 存在，主键正确 | 任意 | 任意 | 验证主键及数据后返回；遗留表保留并报告 |
| 存在，主键缺失 | 不存在 | 不存在 | 读取并检查重复 key；建立临时表、复制、比较后开始交换 |
| 存在，主键缺失 | 存在 | 不存在 | 验证临时表与源表一致后续接；不一致则失败保留两表 |
| 不存在 | 存在且验证通过 | 存在 | 比较备份与临时内容，完成第二次 rename |
| 不存在 | 不存在/无效 | 存在 | 恢复 backup 为 options，返回明确错误，不继续服务 |
| 其他组合 | 任意 | 任意 | 明确报出非秘密状态，保留表并停止自动修复 |

TestDM8OptionMigrationRecovery 在每个状态手工建立确定性夹具，调用真实迁移，核对全部原始记录仍存在、成功目标有唯一主键；模拟复制失败/第二次 rename 失败并验证再次启动可恢复。测试不依赖 SIGKILL 时机。

- [ ] **Step 6: 验证完整启动与重复启动。** TestDM8StartupMigration 在空的隔离 schema 使用真实 InitDB/InitLogDB；记录首次写入数据，再运行迁移两次。用现有 migrationSQLRecorder 检查第二次无重复 DDL，读回配置、token、会话、充值和日志代表性数据。原有 options 错误被记录后继续的路径必须被 DM 前/后置校验挡住。

```sh
TEST_DATABASE_MATRIX=1 GOWORK=off go test ./model -run '^Test(DM8(IndexMetadata|QuotaSchema|PrefillUniqueness|OptionMigrationRecovery|StartupMigration|BusinessContracts)|MigrationSchemaStability)$' -count=1 -v
git diff --check
git commit -m "fix(db): make DM8 schema migration repeatable and recoverable"
```

### Task 7: 主库/日志组合与现有业务回归

**Files:** Modify `model/dm8_integration_test.go`、`.env.example`；必要时按实际失败修改 `model/log.go`、`model/audit_log.go` 的 DM 分支。现有 TestMain 和上游 CI 不变。

**Interfaces:** Consumes 完整 InitDB/InitLogDB 和全部 DM 适配；Produces `TestDatabaseBusinessMatrix`、`TestDM8LogDatabaseMatrix` 两个可直接运行的测试入口。

- [ ] **Step 1: 将任务 5 的行为套件扩展到真实三库。** TestDatabaseBusinessMatrix 按 sqlite/mysql/postgres/dm 子测试依次创建独立数据库连接，以 useDatabaseTestState 切换全局状态，运行同一组关键业务断言；SQLite 使用 t.TempDir 的真实文件，不能只在 DM 上验证改动后的公共代码。

各方言使用独立测试库，先通过真实启动建表，再跑行为子测试；不要仅设置 TEST_MYSQL_DSN 就宣称原来在 SQLite TestMain 下的普通用例变成 MySQL 测试。加入数据库版本日志，DSN 不写日志。

- [ ] **Step 2: 测试日志矩阵。** TestDM8LogDatabaseMatrix 固定九个子测试名：dm_shared、dm_dm、dm_mysql、dm_postgres、dm_sqlite、mysql_dm、postgres_dm、sqlite_dm、dm_clickhouse，分别覆盖 DM/共库、DM/独立 DM、DM/MySQL、DM/PostgreSQL、DM/SQLite、三种已有主库/DM，以及 DM/ClickHouse 日志。独立日志 DSN 由 TEST_LOG_DM_DSN、TEST_LOG_MYSQL_DSN、TEST_LOG_POSTGRES_DSN、TEST_CLICKHOUSE_DSN 提供，使用与主库不同的隔离库/schema。独立 DM 日志的实际 schema 必须精确等于 TEST_LOG_DM_SCHEMA、有 NEWAPI_TEST_ 前缀且不同于 TEST_DM_SCHEMA，写入前验证；其他日志实例也要核对当前测试库身份。

每个组合在 LOG_DB 写入唯一 request_id 的 Log/AuditLog，通过现有查询接口读回、按时间过滤及分页、删除符合条件的旧日志，再验证主库 User/Channel 记录未被影响。主库和日志 schema 中放置同名字段，核对引用始终按各自方言生成。

```go
entry := Log{RequestId: "dm8-log-matrix", UserId: 7101, Content: "中文🙂"}
require.NoError(t, createLog(&entry))
var saved Log
require.NoError(t, LOG_DB.Where("request_id = ?", entry.RequestId).First(&saved).Error)
assert.Equal(t, entry.Content, saved.Content)
```

ClickHouse 的 ID/排序/清理保持现有语义，不使用关系数据库主键期望套用；优先复用现有 clickhouse_log_test.go 用例。测试失败先定位驱动/方言，不将日志写入改到主库绕过。

- [ ] **Step 3: 增加文档化配置示例。** 只在 .env.example 的 SQL_DSN/LOG_SQL_DSN 段追加注释，保留原例子。

```dotenv
# DM8（先按已验证的实例配置建库；特殊字符密码需 URL 编码）
# SQL_DSN=dm://app_user:example_password@127.0.0.1:5236?schema=APP_SCHEMA
# LOG_SQL_DSN=dm://log_user:example_password@127.0.0.1:5236?schema=APP_LOG_SCHEMA
```

- [ ] **Step 4: 验证并提交。**

```sh
TEST_DATABASE_MATRIX=1 GOWORK=off go test ./model -run '^Test(DatabaseBusinessMatrix|DM8LogDatabaseMatrix)$' -count=1 -v
git diff --check
git commit -m "test(db): verify DM8 main and log database combinations"
```

### Task 8: 发布版升级、ARM 交付与严格验收入口

**Files:** Create `scripts/test-dm8-matrix.sh`；Update 本计划“执行证据”；Read `Dockerfile`、`Makefile`、上游最新发布版迁移代码。仅当需要构建修复时修改明确受影响文件。

**Interfaces:** Consumes 任务 1—7 的固定驱动与通过的测试入口；Produces 可审计的四库升级记录、ARM 实际运行记录、无 Skip 的验证结果及受控的剩余改动清单。

- [ ] **Step 1: 增加严格矩阵运行脚本。** 使用 bash，脚本不包含密码、不使用 set -x，先检查环境再执行。脚本中测试名只引用本计划已定义的入口。

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
for name in TEST_MYSQL_DSN TEST_POSTGRES_DSN TEST_DM_DSN TEST_DM_SCHEMA \
    TEST_LOG_DM_DSN TEST_LOG_DM_SCHEMA TEST_LOG_MYSQL_DSN TEST_LOG_POSTGRES_DSN TEST_CLICKHOUSE_DSN; do
    if [[ -z "${!name:-}" ]]; then
        printf 'Missing required test setting: %s\n' "$name" >&2
        exit 1
    fi
done
export TEST_DATABASE_MATRIX=1
export GOWORK=off
result_file=$(mktemp "${TMPDIR:-/tmp}/new-api-dm8-tests.XXXXXX")
trap 'rm -f "$result_file"' EXIT
test_status=0
go test -json -count=1 ./model \
    -run '^Test(DM8|DatabaseBusinessMatrix|MigrationSchemaStability)' >"$result_file" || test_status=$?
python3 - "$result_file" "$test_status" <<'PY'
import json, sys
events = [json.loads(line) for line in open(sys.argv[1]) if line.strip()]
skipped = [event.get('Test', event.get('Package')) for event in events if event.get('Action') == 'skip']
failed = [event.get('Test', event.get('Package')) for event in events if event.get('Action') == 'fail']
required = {'TestDatabaseBusinessMatrix', 'TestDM8LogDatabaseMatrix', 'TestDM8StartupMigration'}
required.update('TestDatabaseBusinessMatrix/' + name for name in ('sqlite', 'mysql', 'postgres', 'dm'))
required.update('TestDM8LogDatabaseMatrix/' + name for name in (
    'dm_shared', 'dm_dm', 'dm_mysql', 'dm_postgres', 'dm_sqlite',
    'mysql_dm', 'postgres_dm', 'sqlite_dm', 'dm_clickhouse'))
passed = {event.get('Test') for event in events if event.get('Action') == 'pass'}
if int(sys.argv[2]) or failed or skipped or not required <= passed:
    print('Database verification incomplete:', 'failed=', failed,
          'skipped=', skipped, 'missing=', sorted(required-passed))
    sys.exit(1)
print('Required database suites passed without skips.')
PY
```

失败时输出失败用例名及缺失组合；需要进一步诊断时按该用例单独运行并保留经过脱敏的记录。脚本不直接倾倒可能包含参数的全部 SQL 日志，测试名也不包含 DSN 或秘密。对于单个开发测试可保留 Skip；该发布入口不接受任何 Skip。

- [ ] **Step 2: 用真实发布版建立升级夹具。** 只读查询上游 release，固定 tag 与 commit，不以当前 HEAD 冒充最新发布版。为 SQLite/MySQL/PostgreSQL 分别用该 tag 启动创建代表性数据库，写入用户、token、配置、订单和日志后关闭旧程序；对备份克隆运行当前迁移至少两次，核对数据、索引、约束、唯一性及二次 DDL。

DM 初始没有上游原生旧版本，因此分开记录：本适配约定的前一 schema 夹具升级，以及发布版代表性数据按已核对映射导入 DM 后的升级。测试导入保留 ID 并检查下一次自增插入；不要把手工建几列称为“发布版升级已验证”。数据迁移工具的实际来源/命令记录在证据中，不把生产搬迁加入本次执行。

- [ ] **Step 3: 执行项目检查与真实矩阵。** 先使用 Bun 构建前端产物，确保根包 embed 的 web/dist 存在；若仅做后端 CI 编译，使用现有 CI 的占位策略并清楚标注，不能将该占位二进制交付部署。

```sh
# 在 web/ 目录执行；完成后回到仓库根目录。
bun install --frozen-lockfile
bun run build
```

```sh
GOWORK=off go vet ./...
GOWORK=off go build ./...
make test
bash scripts/test-dm8-matrix.sh
```

若变更触及 relaykit 公共 API，独立目录中额外执行 `GOWORK=off go build ./...`；正常方案不应触及该模块。确认共享公共路径受影响的 MySQL/PostgreSQL 最低版本要求是否触发并记录结论。

- [ ] **Step 4: 目标 ARM 运行验收。** 只有任务 1 确认 Linux ARM64 后才使用以下命令；将产物交付到隔离验收环境，实际连接 DM8。不得把交叉编译等同于执行通过。

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOWORK=off go build -o /tmp/new-api-dm8-linux-arm64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GOWORK=off go test -c ./model -o /tmp/new-api-model-linux-arm64.test
```

目标机器运行模型测试二进制的 DM 测试及应用：完成初始化、登录/注销、渠道与 token 管理、一次代理请求、日志查询、一次测试充值幂等调用和两次重启；账务测试使用测试订单，不调用真实支付。实际 OS、CPU、DM 构建号和命令逐项记录。

- [ ] **Step 5: 审核最小改动与完成证据。** 以真实上游基线执行下列检查，逐一解释新增的业务文件改动。不自动创建 PR、部署生产或搬迁数据。

```sh
git diff --check
git diff --stat 6e9de44a7
git diff --name-only 6e9de44a7
go list -m gorm.io/gorm gorm.io/driver/mysql gorm.io/driver/postgres
```

若 required DB/ARM 检查无法执行，记录具体缺少的资源和已完成部分，保持对应任务未完成，不将计划复选框全部勾选。

- [ ] **Step 6: 验收通过后提交。** `git commit -m "test(db): gate DM8 delivery on real database and ARM checks"`。最终交接给出精确环境、实际命令/结果、支持的配置组合、升级路径、回退限制及尚未验证的范围。

## 设计覆盖与计划自检

| Spec 要求 | 负责任务 |
| --- | --- |
| 减少上游改动、固定驱动和现有依赖 | 1、2、8 |
| DM 连接、配置、错误脱敏、类型和标识符 | 1、2、4 |
| 冲突写入、RowsAffected、ID 回填 | 1、3 |
| 邮箱锁、认证、任务、账务不变量 | 3、5、7 |
| 单一生产迁移清单、唯一性与可恢复升级 | 6、8 |
| 主库/共库/独立日志/ClickHouse | 7、8 |
| 四库真实验证、发布版升级、ARM 实际运行 | 1、6、7、8 |
| 后续 merge 审核清单 | 8 和 spec 第 11 节 |

计划编写阶段检查：

- [x] 每项 spec 要求有对应任务。
- [x] 区分已存在函数、计划新增接口、驱动验证结果，未将未知官方包 API 写成已确定实现。
- [x] 五项 Review Focus 都有明确输入、预期与负责测试。
- [x] 不复制生产迁移清单；测试夹具清单仅服务测试。
- [x] 代码块与命令是执行说明，未声称运行通过。
- [ ] 用户评审本计划并选择执行方式。

## 执行证据

### 环境与驱动（任务 1，已实测）

- 宿主机：Darwin arm64，go1.25.14 darwin/arm64，CGO_ENABLED=1（宿主）；源码 commit `b9dddc29e50f7c1b3652bb4136728655e0fa795a`。
- DM8 实例：Docker 容器 `dm8`（镜像 `qinchz/dm8-arm64`，**linux/arm64**），宿主映射 `127.0.0.1:5236`。版本 `version: 03134284094-20230927-203585-20067 Pack1`。
- 实例参数（实测）：`COMPATIBLE_MODE=0`（none，非 MySQL 模式）、`CASE_SENSITIVE=0`、`UNICODE=1`(UTF-8)、`LENGTH_IN_CHAR=1`、`PAGE_SIZE=16`。
- 驱动：`github.com/godoes/gorm-dameng v0.6.0`（纯 Go，内嵌 dm8 驱动，CGO_ENABLED=0 可用），来自本机离线 module cache（无网络，`proxy.golang.org` 超时）。GORM 仍为 v1.25.12，未升级 ORM。官方达梦驱动因无网络未能评估。
- 隔离测试账号：`NEWAPI_TEST_APP`（schema 前缀 `NEWAPI_TEST_`，密码含特殊字符 `#`，DSN 中编码为 `%23`，实测连接与 `CURRENT_SCHEMA` 校验通过）。

### 已实测通过的驱动缺口与修复

- `JSON` 基类被 DM8 拒绝（`非法的基类名[JSON]`）→ 方言 `DataTypeOf` 将 json/longtext/mediumtext 映射为 `CLOB`；CLOB 实测可读写 nil/空串/中文/emoji/200KB 大字段。
- `LENGTH_IN_CHAR=1` 下驱动按字节报告 VARCHAR 长度导致每次迁移重复 `ALTER MODIFY` → 启用 `VarcharSizeIsCharLength=true`（生成 `VARCHAR(n CHAR)`），二次迁移不再改列。
- 时间列 DM 报告为 `DATETIME WITH TIME ZONE`，驱动 `TIMESTAMP WITH TIME ZONE` 触发重复 alter → `DataTypeOf` 对 `schema.Time` 返回 `DATETIME WITH TIME ZONE`（实测 DDL 接受且回读一致）。
- `DEFAULT false/true` 被 DM 拒绝（`DEFAULT约束表达式无效`，-2670，导致**重启致命失败**）→ 覆写 `Explain` 将 bool 渲染为 `1/0`。
- `UNIX_TIMESTAMP()` 在 COMPATIBLE_MODE=0 下不支持（-2207）→ `dm8DatabaseTimestamp` 用 `DATEDIFF(SS, TIMESTAMP'1970-01-01 00:00:00', SYS_EXTRACT_UTC(SYSTIMESTAMP))`，实测返回正确 UTC epoch。
- 反引号标识符被 DM 拒绝，`group/key/trade_no/base_url/models` 需双引号 → `initCol` 及 channel/topup/subscription/redemption 的显式引用增加 DM 双引号分支；布尔字面量用 `1/0`（`= false` 报 -6105）。
- `EXISTS (?) AS alias` 被 DM 拒绝（-2007）→ `login_verification.go` 对 DM 改用 `CASE WHEN EXISTS (?) THEN 1 ELSE 0 END AS alias`。
- GORM `Updates` 对自增列发出 `SET "id"=?`，DM 禁止修改标识列（-2664）→ `EditWithTx` 的 `Omit` 增加 `id`。
- 驱动 `GetIndexes` 将 `NON_UNIQUE` 误赋给 `UniqueValue`（唯一索引报告为非唯一）→ 唯一生产消费者 `optionsKeyIsUnique` 通过 `PrimaryKey()` 判定，实测 options 主键识别正确；未包装 Migrator（驱动内部对 `db.Migrator().(dameng.Migrator)` 做类型断言，包装会 panic）。
- `OnConflict` 非主键唯一列：驱动退化为普通 INSERT，冲突报 -6625（重启时 authz 种子致命失败）→ 新增 `registerDM8Create`/`createDM8WithConflict`：先跑驱动原回调，仅在 -6625 时接管；DoNothing 单行置 RowsAffected=0、批量逐行重放；DoUpdates 用 INSERT 命中冲突后按冲突列 WHERE 执行 UPDATE。实测事务内 -6625 后可继续（DM 不因单语句错误回滚整个事务）。
- 软删除部分唯一索引（`where: deleted_at IS NULL`）：GORM 通用 CreateIndex 忽略 Where → `ensureDM8PrefillUniqueIndex` 迁移后改建 `CASE WHEN deleted_at IS NULL THEN name ELSE NULL END` 表达式唯一索引；实测活跃重名被拒、软删后可重用、多条 NULL 并存；用 `USER_IND_COLUMNS.COLUMN_POSITION<0` 判定函数索引以保证幂等。

### 真实实例验证结果

- `go build ./...`（darwin/arm64，GOPROXY=off 只读模式）通过；`go vet ./model` 通过。
- DM8 驱动 `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build` 通过（纯 Go，ARM 兼容）。**完整 linux/arm64 二进制**因离线缓存缺少 `numcpus@v0.6.1`/`procfs@v0.15.1`（`common/` 监控相关的既有依赖，与 DM8 无关）未能交叉编译。
- `model/dm8_integration_test.go` 6 个用例全部对真实 DM8 通过：StartupMigration（生产 `migrateDB` 连续两次幂等 + 锁锚点 + options 主键识别）、PrefillSoftDeleteUniqueness、ValueRoundTrip、RankingBuckets（FLOOR 分桶）、ConflictUpsert、ConflictDoNothing（含事务内 RowsAffected 契约）。
- 既有回归：`go test ./model/... ./common/... ./service/authz/...`（SQLite，未设 TEST_DM_DSN，DM 用例 Skip）全部通过，无回归。
- 服务实机运行（linux/arm64 容器中的 DM8，宿主 macOS 进程经 TCP 连接）：全新 schema 首次启动完成迁移并监听；注册用户（走 DM 邮箱锁锚点）成功；登录成功（会话/JWT）；创建 API token 成功并持久化；`/api/status` 返回 200；提升为 root 后 `SearchChannels` 双引号查询成功；**重启（第二次启动）无 -6625/-2670、无 FATAL，角色与数据持久化正确**。

### 尚未验证 / 限制

- 官方达梦 Go/GORM 驱动未评估（无网络）；仅验证社区 v0.6.0。
- 未在物理 ARM 主机运行；ARM 验证通过 Apple Silicon 上的 linux/arm64 Docker 容器完成，完整 linux/arm64 应用二进制未交叉编译成功（缺既有无关依赖的离线缓存）。
- MySQL/PostgreSQL/ClickHouse 真实实例矩阵、发布版旧库升级夹具、独立日志库与 DM/ClickHouse 组合（任务 7、8 的完整矩阵）未在本次运行。
- **重要**：本实例 `CASE_SENSITIVE=0`，字符串比较大小写不敏感（实测 `key='mixed-key'` 命中 `'MiXeD-Key'`）。设计第 6.1 节指出该设置同时影响对象名与数据，会使 API key、token_hash、外部身份标识等的精确匹配变为大小写不敏感，存在安全/正确性影响。生产 DM8 实例建议按设计以 `CASE_SENSITIVE=1` 建库；若沿用 `CASE_SENSITIVE=0`，需显式接受该语义并复核所有依赖精确大小写比较的凭证路径。
- 运行时逐端点全覆盖验证（除已测的注册/登录/会话/token/渠道搜索/排行榜/冲突写入外）未穷尽；其余查询按“实测失败再扩展”的原则处理。

## 执行交接

本计划建议 Native：在当前会话按顺序执行，由同一执行者保持驱动、迁移和事务语义的一致理解，最后进行独立审查。各任务前后依赖强，目标是控制对上游的改动面。

也可选择 Subagent-driven：每任务由新的执行子代理及独立审查者处理，审查粒度更细，但上下文与协调开销更高。此处只是提供选择，不是已经获得启动子代理或实施的指令。

按 writing-plans 的 Execution Handoff，用户评审计划并选择执行方式后才进入实施；任务 1 所需的实例与官方包仍须在执行时落实。
