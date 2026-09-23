# 开发手册（精简版）

new-api 是**单二进制、前后端一体**：前端 React 构建为静态文件后由 `//go:embed web/dist` 内嵌进 Go 二进制，运行时由后端 :3000 一并托管，**无需单独启动前端**。

## 环境

- 数据库：达梦 DM8（Docker 容器 `dm8`，linux/arm64），宿主映射 `127.0.0.1:5236`
- 应用账号：`LDW_AI_AGG`（schema 同名），配置在项目根目录 `.env`（已 gitignore）
- 管理后台：`http://localhost:3000/`，账号 `admin` / `Passw0rd!23`（role=100）
- 包管理：前端用 `bun`；Go 依赖已在本机缓存，离线可编译

## 启动前

确认 DM8 容器在跑：

```bash
docker ps --filter name=dm8        # 没跑就: docker start dm8
```

## 日常启动（按改动类型选）

先停旧进程，避免端口占用：

```bash
lsof -ti tcp:3000 | xargs kill -9
```

| 改了什么 | 命令 |
|---|---|
| 只改后端 `.go` | `go run .` |
| 改了前端 `web/` | `cd web && bun run build` → 回根目录 `go run .` |
| 只改 `.env` 配置 | `go run .`（仅重启） |

```bash
cd /Users/tiger/GoProject/new-api && go run .
```

> 前端改动必须先 `bun run build` 再 `go run .`，顺序不能反（静态文件需重新内嵌）。

## 换端口 / 后台运行

```bash
PORT=3001 go run .                                  # 临时换端口
nohup ./new-api > run.log 2>&1 &                    # 后台（先 go build -o new-api .）
```

## 配置（.env）

关键项，改完重启即可：

```dotenv
SQL_DSN=dm://LDW_AI_AGG:LdwAiAgg_2026@127.0.0.1:5236?schema=LDW_AI_AGG
PORT=3000
SESSION_SECRET=<随机串>
```

- 密码中若含 URL 特殊字符需编码（如 `#` → `%23`）；当前密码无特殊字符，无需转义
- 不设 `SQL_DSN` 时回退 SQLite（本地文件库）

## 测试

```bash
# 既有回归（SQLite，DM 用例自动 Skip）
go test ./model/... ./common/...

# DM8 集成测试（需真实 DM8）
TEST_DM_DSN='dm://NEWAPI_TEST_APP:NewApi%23Test123@127.0.0.1:5236?schema=NEWAPI_TEST_APP' \
TEST_DM_SCHEMA=NEWAPI_TEST_APP \
go test ./model -run '^TestDM8' -count=1
```

## 常见坑

- **address already in use**：旧进程没杀干净，`lsof -ti tcp:3000 | xargs kill -9`
- **首页显示 placeholder**：前端没构建，`cd web && bun run build` 后重新 `go run .`
- **从非根目录启动读不到 .env**：`.env` 在项目根，务必 `cd` 到根目录再启动
- **离线装 Go 依赖失败**：加 `GOFLAGS=-mod=mod GOPROXY=file:///Users/tiger/go/pkg/mod/cache/download GOSUMDB=off`
- **bun install 报 IntegrityCheckFailed**：删除 `~/.bun/install/cache` 下对应损坏包目录后重装（npm 源可达）

## DM8 适配要点（改数据库相关代码前必读）

详见 `docs/superpowers/plans/2026-09-21-dm8-minimal-integration.md` 的「执行证据」。核心：

- 实例为 `CASE_SENSITIVE=0`、`COMPATIBLE_MODE=0`、UTF-8、`LENGTH_IN_CHAR=1`
- DM 专用逻辑集中在 `model/dm8*.go`；其余文件仅保留必要的 DM 短分支
- 标识符用双引号（反引号在 DM 报错）；布尔字面量用 `1/0`
- **聚合/表达式列的别名必须加双引号**（如 `Select(`sum(x) "total"`)`）：DM 会把未加引号的别名转成大写，导致 GORM 扫描进结构体失败（单列时尤其）
- `OnConflict` 非主键唯一列由 `model/dm8_create.go` 适配
- 生产建库建议 `CASE_SENSITIVE=1`（当前实例为 0，字符串比较大小写不敏感）
