# 高性能图片画廊微服务

一个基于 **Rust (Axum + SQLx + PostgreSQL)** 开发的、高度可靠且具备高并发性能的容器化画廊微服务。系统集成了流式不占内存上传、原子化文件路径事务、高维感知哈希（aHash、dHash 与 pHash）去重验证、分桶哈希查询索引，以及全自动异步后台垃圾文件清理。

---

## 核心设计亮点

1. **原子化文件持久化**：
   图片在上传时，一边进行 Multipart 流式读取，一边流式计算其 SHA-256 指纹，写入 `data/tmp/` 临时文件。只有当临时文件完美 `rename` 移动到二级分级物理路径 (`data/images/<h1:4>/<h2:4>/<h3:rest>.<ext>`) 成功后，才正式提交数据库事务，避免因意外崩溃引发“数据库指向幽灵图片”的灾难性不一致。
2. **全局并发写锁**：
   写操作（如上传、新建多对多关联）采用 `Arc<tokio::sync::Mutex<()>>` 在应用层进行写操作串行化，彻底防范并发竞争状态。计算密集型（解码、感知哈希抽取）任务在锁外完成，最大化系统吞吐量。
3. **高维感知去重指纹**：
   - **静态图片**：结合 64位 `aHash`、128位水平与垂直双向 `dHash`、64位基于离散余弦变换（DCT）低频系数二值化的 `pHash` 进行多层次相似度计算。
   - **GIF 动图**：对 GIF 进行多达 5 帧的均匀采样，每帧抽取 dHash，最终利用异或 (XOR) 逻辑聚合生成动图综合感知指纹。
   - **分桶检索指纹**：提取 128位 dHash 水平部分的 4 段 16-bit 整数作为分桶键（`bucket1` - `bucket4`），将对比候选集极限缩减至 `~0.006%`，支撑百万级大图毫秒级对比。
4. **全自动异步后台清理**：
   内置定时后台 Worker，周期性自动扫描并抹除未关联任何画廊的“孤儿图片”记录、以及存活超过 24 小时的临时垃圾。

---

## 项目工程结构

```
gallery-service/
├── Cargo.toml
├── migrations/
│   └── 001_init.sql          -- 数据库 Schema、级联规则与索引
├── src/
│   ├── main.rs               -- 系统入口绑定与主生命周期
│   ├── config.rs             -- 环境变量加载与挂载点同源原子性校验
│   ├── error.rs              -- 统一错误类型映射（规范精确 400/409 拦截）
│   ├── state.rs              -- 共享应用上下文、连接池与全局写锁
│   ├── db.rs                 -- 感知哈希分桶碰撞过滤引擎
│   ├── models.rs             -- 数据库实体与 DTO 请求响应参数
│   ├── background.rs         -- 后台定时异步清理调度器
│   ├── storage.rs            -- 二级分片原子路径移动工具
│   └── hash/
│       ├── mod.rs
│       ├── sha256.rs         -- 流式不占内存 SHA-256 累加器
│       └── perceptual.rs     -- aHash, dHash, DCT pHash 纯 Rust 算法模块
│   └── routes/
│       ├── mod.rs
│       ├── gallery.rs        -- 画廊 CRUD 与全局别名控制端点
│       ├── image.rs          -- 全局图片检索与别名分配端点
│       ├── gallery_image.rs  -- 核心 Multipart 图片流上传与去重关联
│       ├── file.rs           -- 物理图片实体流高效传输服务
│       └── refresh.rs        -- 手动运维刷新与触发接口
└── docker-compose.yml        -- 容器化生产编排
```

---

## 快速开始

### 1. 环境准备

- Docker 与 Docker Compose (v2.0+)
- Postgres 客户端工具（若想要在宿主机独立编译调试）

### 2. 部署服务

你可以直接根据我们为你准备的环境变量模板配置 `.env` 文件（参见 `.env.example`）：
```bash
cp .env.example .env
# 修改 .env 中的 POSTGRES_PASSWORD
```

一键式拉起容器集群：
```bash
docker compose up -d --build
```
系统会瞬间自动初始化 PostgreSQL 16 数据库并完成全自动 schema 迁移，同时在 `3000` 端口提供服务。

### 3. 健康检查

通过 E2E 健康检查探针确认服务可用：
```bash
curl -i http://localhost:3000/health
```

---

## Web API 端点规范

### 画廊管理
- `POST /galleries` - 创建画廊 `{ "name": "...", "aliases": ["..."] }`
- `GET /galleries?search=keyword` - 过滤列表（支持模糊匹配名称及别名）
- `GET /galleries/:id` - 查询画廊属性详情
- `PATCH /galleries/:id` - 改名
- `DELETE /galleries/:id` - 物理删除画廊（级联删除别名，图片物理保留）
- `POST /galleries/:id/aliases` - 增加全局唯一别名
- `DELETE /galleries/:id/aliases/:alias` - 移出别名

### 图片全局维护
- `GET /images?search=keyword&gallery_id=uuid` - 全局图片筛选
- `GET /images/:id` - 查询图片底层属性与分配别名列表
- `GET /images/by-hash/:prefix` - 利用 SHA-256 十六进制前缀唯一匹配详情（不唯一返回 409）
- `PATCH /images/:id` - 修改图片名称及 JSONb 格式 metadata 字段
- `POST /images/:id/aliases` - 分配图片别名
- `DELETE /images/:id/aliases/:alias` - 移除图片别名

### 画廊内图片操作
- `POST /galleries/:gallery_id/images` - Multipart 流上传图片（`file` 字段）。支持可选参数 `?force=true` 强制绕过相似排重。
- `GET /galleries/:gallery_id/images` - 分页或全量检索画廊内图片
- `DELETE /galleries/:gallery_id/images/:image_id` - 从画廊移除关联

### 静态文件与运维任务
- `GET /files/:image_id` - 获取图片的原始物理资产文件流
- `POST /refresh` - 手动强制调度后台清理器清理孤儿或临时碎屑 `{ "clear_orphans": true, "clear_temp": true, "refresh_hashes": false }`
