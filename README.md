# gallery-server

一个基于 **Gin + PostgreSQL + SeaweedFS + Imaginary + Qdrant + Valkey** 的图片图库服务。

它提供：标签管理、图片上传、元数据查询、按标签筛选、图片描述维护，以及基于 Imaginary 的在线渲染。上传链路会把图片统一转换为 **lossless WebP**，并结合 **pHash** 与 **向量相似度** 做去重。

## 功能概览

- **标签管理**
  - 创建标签
  - 标签名大小写不敏感唯一（`Cat` 和 `cat` 视为同一个标签）
  - 支持按关键字模糊查询
- **图片上传**
  - 接收 multipart 上传
  - 自动转为 **lossless WebP** 存储
  - 自动提取宽高、动画标记、pHash、256 维灰度向量
  - 上传时可关联已存在标签
- **重复图片检测**
  - 先做 **pHash 精确检查**
  - 再做 **Qdrant 向量相似度检查**
  - 可通过 `force=true` 跳过重复检查
- **图片查询**
  - 分页列出图片
  - 支持按多个标签筛选（**AND 语义**）
  - 获取单张图片元数据
- **描述维护**
  - 为图片设置、覆盖或清空描述
- **图片渲染**
  - 通过 Imaginary 做缩放、质量调整、格式转换
  - 不传转换参数时直接返回原图
- **认证**
  - 区分读权限和写权限
  - 支持 `Authorization: Bearer`、自定义 Header、`token` 查询参数
- **工程配套**
  - 提供 `OpenAPI` 描述
  - 提供 `Dockerfile`、`docker-compose.yml`
  - 提供 `Makefile` 与 compose smoke test
  - GitHub Actions CI 已配置

---

## 系统架构

```text
Client
  │
  ▼
Gin Router
  ├─ RequestID 中间件
  ├─ 读鉴权 / 写鉴权
  └─ Handlers
       │
       ▼
    Services
       ├─ TagService
       └─ ImageService
            ├─ PostgreSQL   -> 图片/标签元数据
            ├─ Valkey       -> 上传去重锁
            ├─ Imaginary    -> 转 WebP / 在线渲染
            ├─ SeaweedFS    -> 图片对象存储
            └─ Qdrant       -> 相似图向量索引
```

### 上传链路

1. 校验请求参数、文件大小、标签是否合法。
2. 通过 Valkey 对原始文件内容的 SHA-256 建锁，避免同一文件并发重复处理。
3. 调用 Imaginary 把输入图片转换为 **lossless WebP**。
4. 计算：
   - `pHash`
   - 256 维灰度向量
   - 宽高
   - 是否动画 WebP
5. 如果未设置 `force=true`：
   - 先检查 PostgreSQL 中是否存在相同 `pHash`
   - 再检查 Qdrant 中是否存在超过阈值的相似向量
6. 将 WebP 上传到 SeaweedFS。
7. 在 PostgreSQL 中写入图片元数据与标签关联。
8. 将向量写入 Qdrant。

---

## 目录结构

```text
cmd/server/                 程序入口
internal/app/               应用装配
internal/config/            环境变量配置
internal/http/              路由、中间件、HTTP handlers
internal/service/           业务逻辑
internal/repository/postgres/ PostgreSQL 仓储实现
internal/clients/           PostgreSQL / Valkey / Qdrant / SeaweedFS / Imaginary 客户端
internal/imagehash/         pHash 与向量提取
internal/model/             数据模型
internal/port/              端口接口定义
migrations/                 SQL 迁移
openapi/openapi.yaml        OpenAPI 描述
scripts/compose-smoke-test.sh Docker Compose 烟雾测试
```

---

## 运行要求

### 本地开发

- Go **1.26.1**
- PostgreSQL
- Valkey
- Qdrant
- SeaweedFS（master + volume，filer 可选）
- Imaginary

### 推荐方式

**最推荐直接使用 Docker Compose 启动整套依赖和应用。**

原因：`.env.example` 里的默认地址使用的是 Compose 内部服务名（如 `postgres`、`valkey`、`qdrant`），最适合容器内互联。

---

## 快速开始（Docker Compose）

### 1）准备环境变量

```bash
cp .env.example .env
```

按需修改：

- `READ_TOKEN`
- `WRITE_TOKEN`
- `APP_PORT`

> 默认情况下，`docker-compose.yml` 只对外暴露应用端口；PostgreSQL / Valkey / Qdrant / SeaweedFS / Imaginary 的宿主机端口映射默认注释掉了。

### 2）启动服务

```bash
docker compose up --build -d
```

### 3）检查健康状态

```bash
curl http://localhost:8080/healthz
```

预期：

```json
{"status":"ok"}
```

### 4）停止服务

```bash
docker compose down
```

如果想连数据卷一起删除：

```bash
docker compose down -v
```

---

## 本地直接运行（Go）

如果你不使用 Compose 运行应用本身，而是在宿主机直接 `go run`：

1. 先确保所有依赖服务可访问；
2. 把 `.env.example` 中的服务地址改成宿主机可访问地址，例如 `localhost:5432`；
3. 手动导出环境变量后启动。

示例：

```bash
set -a
source .env
set +a

go run ./cmd/server
```

构建可执行文件：

```bash
go build -o gallery-server ./cmd/server
```

---

## 配置说明

应用通过环境变量读取配置，`config.Load()` **不会自动加载 `.env` 文件**；
`.env` 只是给 Docker Compose 使用的。若本地直接运行，请自行导出环境变量。

### 服务与认证

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `SERVER_ADDR` | `:8080` | HTTP 监听地址 |
| `GIN_MODE` | `release` | Gin 运行模式 |
| `SERVER_SHUTDOWN_TIMEOUT` | `10s` | 优雅关闭超时 |
| `READ_TOKEN` | 空 | 读接口令牌；为空时读接口开放 |
| `WRITE_TOKEN` | 空 | 写接口令牌；为空时写接口开放 |

### PostgreSQL

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `POSTGRES_DSN` | `postgres://gallery:gallery@postgres:5432/gallery?sslmode=disable` | 连接串 |
| `POSTGRES_MAX_CONNS` | `10` | 连接池上限 |
| `POSTGRES_AUTO_MIGRATE` | `true` | 启动时自动执行 `migrations/*.sql` |
| `POSTGRES_MIGRATIONS_DIR` | `migrations` | 迁移目录 |

### Valkey

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `VALKEY_ADDR` | `valkey:6379` | Valkey 地址 |
| `VALKEY_PASSWORD` | 空 | 密码 |
| `VALKEY_DB` | `0` | 数据库编号 |
| `VALKEY_LOCK_TTL` | `30s` | 上传锁 TTL |

### Imaginary

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `IMAGINARY_BASE_URL` | `http://imaginary:9000` | Imaginary 基地址 |
| `IMAGINARY_TIMEOUT` | `60s` | 请求超时 |

### SeaweedFS

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `SEAWEED_MASTER_URL` | `http://seaweed-master:9333` | 分配 FID 使用 |
| `SEAWEED_PUBLIC_URL` | `http://seaweed-volume:8080` | 对外文件访问基地址 |
| `SEAWEED_UPLOAD_TIMEOUT` | `60s` | 上传/删除超时 |

### Qdrant

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `QDRANT_GRPC_ADDR` | `qdrant:6334` | gRPC 地址 |
| `QDRANT_API_KEY` | 空 | API Key |
| `QDRANT_COLLECTION` | `image_vectors` | 向量集合名 |
| `QDRANT_TIMEOUT` | `20s` | 连接/请求超时 |
| `QDRANT_SIMILARITY_THRESHOLD` | `0.98` | 相似图阈值 |
| `QDRANT_SEARCH_LIMIT` | `5` | 相似图搜索上限 |

### 上传与分页

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `UPLOAD_MAX_BYTES` | `33554432` | 最大上传大小，默认 32 MiB |
| `PAGINATION_DEFAULT_PAGE_SIZE` | `20` | 默认分页大小 |
| `PAGINATION_MAX_PAGE_SIZE` | `100` | 最大分页大小 |

---

## 认证规则

### Header / Query 支持

服务支持以下令牌传递方式：

1. `Authorization: Bearer <token>`
2. `X-Read-Token: <token>`
3. `X-Write-Token: <token>`
4. `X-API-Token: <token>`
5. `?token=<token>`

读取顺序优先级大致为：

```text
Authorization Bearer > 专用 Header > X-API-Token > token 查询参数
```

### 权限规则

- 当 `READ_TOKEN` 为空时：**读接口开放**
- 当 `WRITE_TOKEN` 为空时：**写接口开放**
- 当读写 token 都配置时：
  - 读接口接受 `READ_TOKEN`
  - 读接口也接受 `WRITE_TOKEN`
  - 写接口只接受 `WRITE_TOKEN`

---

## API 概览

### 无需认证

- `GET /healthz`

### 读接口

- `GET /v1/tags`
- `GET /v1/images`
- `GET /v1/images/random`
- `GET /v1/images/{id}`
- `GET /v1/images/{id}/render`

### 写接口

- `POST /v1/tags`
- `POST /v1/images/upload`
- `POST /v1/images/{id}/description`

完整接口定义见：[`openapi/openapi.yaml`](openapi/openapi.yaml)

---

## 核心行为说明

### 标签

- 标签创建时会去掉首尾空白。
- 标签最长 **64 个字符**。
- 标签名在数据库中按 **大小写不敏感唯一** 约束保存。
- 上传图片时，**不会自动创建标签**；传入的标签必须已经存在，否则返回 400。

### 图片上传

- 上传字段名固定为 `file`。
- `tags` 可以重复传多个，也可以逗号分隔。
- 存储前会转为 **lossless WebP**。
- 数据库存储的是转换后的：
  - `file_size`
  - `mime_type`（固定为 `image/webp`）
  - 宽高
  - `phash`
  - `is_animated`
- 默认会做重复检测；传 `force=true` 可跳过。
- 如果命中已存在图片，接口会返回 `409`，并尽量附带 `duplicate_image_id` 指向已存在图片。

### 图片筛选

`GET /v1/images` 支持：

- `tag=cat&tag=cover`
- `tags=cat,cover`
- 混合使用

多个标签之间是 **AND** 关系：只有同时拥有所有标签的图片才会返回。

分页响应会额外返回 `total`，表示当前过滤条件下的图片总数。

### 随机图片

`GET /v1/images/random` 支持和 `GET /v1/images` 相同的 `tag` / `tags` 过滤参数。

- 不传过滤条件：从全库随机返回一张图片
- 传 tag：从满足 **AND** 过滤条件的图片中随机返回一张
- 如果没有匹配图片：返回 `404`

### 图片描述

`POST /v1/images/{id}/description`

- 传入非空字符串：设置/覆盖描述
- 传入空字符串或仅空白：清空描述

### 图片渲染

`GET /v1/images/{id}/render`

支持参数：

- `w` / `h`：宽高
- `fit`：`cover` / `contain` / `fill` / `inside` / `outside`
- `quality`：1~100
- `format`：`jpeg` / `jpg` / `png` / `webp` / `auto`

规则：

- 如果 **完全不传任何转换参数**，直接返回原图
- 如果设置了 `fit`，至少要提供 `w` 或 `h`
- 如果只传 `quality` 或 `format`，可以在原尺寸基础上输出

---

## 调用示例

以下示例默认：

```bash
export BASE_URL="http://localhost:8080"
export READ_TOKEN="read-secret"
export WRITE_TOKEN="write-secret"
```

### 1）创建标签

```bash
curl -X POST "$BASE_URL/v1/tags" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $WRITE_TOKEN" \
  -d '{"name":"Cat"}'
```

### 2）查询标签

```bash
curl "$BASE_URL/v1/tags?q=ca&limit=20" \
  -H "Authorization: Bearer $READ_TOKEN"
```

### 3）上传图片

```bash
curl -X POST "$BASE_URL/v1/images/upload" \
  -H "Authorization: Bearer $WRITE_TOKEN" \
  -F "file=@./example.png" \
  -F "tags=Cat" \
  -F "tags=Cover"
```

跳过去重：

```bash
curl -X POST "$BASE_URL/v1/images/upload" \
  -H "Authorization: Bearer $WRITE_TOKEN" \
  -F "file=@./example.png" \
  -F "force=true"
```

如果上传命中重复图，返回示例：

```json
{
  "error": "conflict: an image with the same perceptual hash already exists",
  "duplicate_image_id": 123
}
```

### 4）列出图片

```bash
curl "$BASE_URL/v1/images?page=1&page_size=20&tags=Cat,Cover" \
  -H "Authorization: Bearer $READ_TOKEN"
```

返回示例：

```json
{
  "items": [],
  "page": 1,
  "page_size": 20,
  "total": 0
}
```

### 5）随机获取一张图片

```bash
curl "$BASE_URL/v1/images/random?tags=Cat,Cover" \
  -H "Authorization: Bearer $READ_TOKEN"
```

### 6）获取单张图片元数据

```bash
curl "$BASE_URL/v1/images/1" \
  -H "Authorization: Bearer $READ_TOKEN"
```

### 7）设置图片描述

```bash
curl -X POST "$BASE_URL/v1/images/1/description" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $WRITE_TOKEN" \
  -d '{"description":"一只趴在窗边的猫"}'
```

清空描述：

```bash
curl -X POST "$BASE_URL/v1/images/1/description" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $WRITE_TOKEN" \
  -d '{"description":""}'
```

### 8）渲染缩略图

```bash
curl "$BASE_URL/v1/images/1/render?w=480&h=320&fit=cover&format=jpeg&quality=85" \
  -H "Authorization: Bearer $READ_TOKEN" \
  -o thumb.jpg
```

---

## 数据库与迁移说明

迁移位于 `migrations/` 目录，启动时在 `POSTGRES_AUTO_MIGRATE=true` 的情况下按文件名字典序执行。

当前包含的关键约束：

- `tags.id` 为 `SERIAL`
- `images.id` 为 `BIGSERIAL`
- `image_tags.image_id` 为 `BIGINT`
- `tags` 存在 `LOWER(name)` 唯一索引
- `images.description` 默认空字符串
- `images.phash` 已建立索引

> 当前迁移机制没有单独的 migration table，而是直接执行目录下 SQL 文件，所以迁移脚本需要保持 **可重复执行**。

---

## 开发命令

```bash
make fmt            # 格式化代码
make fmt-check      # 检查格式
make tidy           # go mod tidy
make test           # 运行测试
make build          # 构建二进制
make run            # 本地运行服务
make ci             # fmt-check + tidy + test + build + compose-config
make compose-up     # 启动 compose
make compose-down   # 关闭 compose
make compose-config # 校验 compose 配置
make smoke-compose  # 运行 compose 烟雾测试
```

---

## 测试与 CI

仓库包含：

- 单元测试：
  - `internal/service/*_test.go`
  - `internal/http/*_test.go`
  - `internal/imagehash/phash_test.go`
- GitHub Actions CI：
  - `gofmt` 检查
  - `go mod tidy`
  - `go test ./...`
  - `go build ./cmd/server`
  - `docker compose --env-file .env.example config`
  - `scripts/compose-smoke-test.sh`

---

## OpenAPI

接口定义文件：

- [`openapi/openapi.yaml`](openapi/openapi.yaml)

如果你在做前后端联调、SDK 生成或 API 文档托管，可以直接基于这个文件继续处理。

---

## 目前的接口边界

当前代码中**已经实现**的能力：

- 标签创建 / 查询
- 图片上传
- 图片列表 / 单图查询
- 图片描述写入
- 图片渲染

当前代码中**尚未实现**的常见能力：

- 图片删除
- 标签删除 / 重命名
- 用户体系
- 多租户
- 批量导入/导出
- 后台管理界面

---

## 许可证

仓库中当前未声明许可证。如需开源发布，请补充对应 LICENSE 文件。
