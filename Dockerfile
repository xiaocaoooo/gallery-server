# 阶段 1：构建
FROM rust:slim-bookworm AS builder

WORKDIR /app

# 安装构建依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config libssl-dev libpq-dev curl \
    && rm -rf /var/lib/apt/lists/*

# 先复制依赖文件，利用 Docker 缓存层
COPY Cargo.toml Cargo.lock ./
COPY migrations ./migrations

# 创建虚拟 main.rs 以缓存依赖编译
RUN mkdir src && echo "fn main() {}" > src/main.rs
RUN cargo build --release && rm -rf src

# 复制完整源码并构建
COPY src ./src
RUN touch src/main.rs && cargo build --release

# 阶段 2：运行
FROM debian:bookworm-slim AS runtime

WORKDIR /app

# 安装运行时依赖
RUN apt-get update && apt-get install -y --no-install-recommends \
    libssl3 libpq5 ca-certificates wget \
    && rm -rf /var/lib/apt/lists/*

# 创建非 root 用户
RUN groupadd -r gallery && useradd -r -g gallery gallery

# 复制二进制文件和迁移
COPY --from=builder /app/target/release/gallery-service /usr/local/bin/gallery-service
COPY --from=builder /app/migrations ./migrations

# 创建数据目录并设置权限
RUN mkdir -p /app/data/tmp /app/data/images && chown -R gallery:gallery /app/data

USER gallery

EXPOSE 3000

ENV DATA_DIR=/app/data
ENV TMP_DIR=/app/data/tmp
ENV RUST_LOG=info
ENV LOG_FORMAT=json
ENV BIND_ADDRESS=0.0.0.0:3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:3000/health || exit 1

ENTRYPOINT ["/usr/local/bin/gallery-service"]
