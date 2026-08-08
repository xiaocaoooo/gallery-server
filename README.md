# High-Performance Image Gallery Microservice

A containerized, highly performant, and reliable image gallery microservice built with **Rust (Axum + SQLx + PostgreSQL)**. It integrates stream-based uploads, atomic file path transactions, high-dimensional perceptual image hashing (aHash, dHash, and pHash) for duplicate detection, bucket-based indexes, and asynchronous automated orphan cleanup.

---

## Key Features

1. **Atomic File Path Operations**: 
   Images are streamed into a temporary directory `data/tmp/` while concurrently computing their SHA-256 hash. Temporary files are atomically renamed to their final tiered paths (`data/images/<h1:4>/<h2:4>/<h3:rest>.<ext>`) *before* committing the DB transaction. This sequence prevents orphaned "DB pointer ghost records" in case of crashes.
2. **Global Upload Serialization**:
   All write operations are synchronized using an `Arc<tokio::sync::Mutex<()>>` lock to guarantee safe state execution and eliminate concurrency race conditions. CPU-heavy processing (decoding, hashing) is decoupled outside this lock to optimize throughput.
3. **High-Dimensional Perceptual Duplicate Check**:
   - **Static Images**: Combines 64-bit `aHash`, 128-bit vertical & horizontal `dHash`, and 64-bit `pHash` (DCT-based low-frequency coefficients二值化 via `rustdct`) for similarity validation.
   - **GIF Images**: Samplings of up to 5 uniform frames are merged using XOR aggregation to generate composite perceptual hashes.
   - **Bucket Search Index**: Features 4x16-bit integer indexing to filter and scale down query candidates to `~0.006%` for million-level sub-second matching.
4. **Automated Background Orphan Cleanup**:
   Features a background scheduling worker that periodically sweeps unused images (images not associated with any galleries) and temporary leftovers older than 24 hours.

---

## Project Structure

```
gallery-service/
├── Cargo.toml
├── migrations/
│   └── 001_init.sql          -- PostgreSQL Database Schema & Indexes
├── src/
│   ├── main.rs               -- Service Entrypoint
│   ├── config.rs             -- Environment Configuration & Atomicity Check
│   ├── error.rs              -- Unified API Errors with accurate Http Status
│   ├── state.rs              -- Shared AppState with connection pool & lock
│   ├── db.rs                 -- Perceptual similarity transaction query
│   ├── models.rs             -- Database entities & DTO structures
│   ├── background.rs         -- Automated scheduling worker
│   ├── storage.rs            -- Atomic tiered file system persistency
│   └── hash/
│       ├── mod.rs
│       ├── sha256.rs         -- Non-blocking stream-based SHA-256 computation
│       └── perceptual.rs     -- Perceptual hashing (aHash, dHash, DCT pHash, GIF sampling)
│   └── routes/
│       ├── mod.rs
│       ├── gallery.rs        -- Gallery CRUD & Aliases management
│       ├── image.rs          -- Global image retrieval & Aliases association
│       ├── gallery_image.rs  -- Multipart Stream Upload with duplicates filter
│       ├── file.rs           -- Original file asset stream delivery
│       └── refresh.rs        -- Manual trigger endpoints
└── docker-compose.yml        -- Production-ready orchestration
```

---

## Getting Started

### 1. Requirements

- Docker and Docker Compose (v2.0+)
- Postgres client / toolchain (if running locally on host)

### 2. Launch Services

Simply set up your configuration profile inside a `.env` file (refer to `.env.example`):
```bash
cp .env.example .env
# Edit POSTGRES_PASSWORD in .env
```

And bring up the Docker cluster:
```bash
docker compose up -d --build
```
This starts the Axum application container and the PostgreSQL 16 container, setting up database tables automatically.

### 3. API Handshake Health Check

Ensure the service is fully up:
```bash
curl -i http://localhost:3000/health
```

---

## API Specification Reference

### Gallery Management
- `POST /galleries` - Create a gallery `{ "name": "...", "aliases": ["..."] }`
- `GET /galleries?search=keyword` - List galleries with search filters
- `GET /galleries/:id` - Fetch gallery details
- `PATCH /galleries/:id` - Update gallery name
- `DELETE /galleries/:id` - Delete gallery (unlink associations, keep physical assets)
- `POST /galleries/:id/aliases` - Add global gallery alias
- `DELETE /galleries/:id/aliases/:alias` - Unbind gallery alias

### Global Images
- `GET /images?search=keyword&gallery_id=uuid` - Filter across stored assets
- `GET /images/:id` - Fetch detailed asset metadata & active aliases
- `GET /images/by-hash/:prefix` - Fetch asset details by SHA-256 hex prefix (returns 409 if ambiguous)
- `PATCH /images/:id` - Update asset name and metadata JSONb fields
- `POST /images/:id/aliases` - Bind alias identifier to an asset
- `DELETE /images/:id/aliases/:alias` - Unbind alias identifier

### Gallery-Image Operations
- `POST /galleries/:gallery_id/images` - Upload file using Multipart (`file` field). Supports optional query `?force=true` to skip perceptual check.
- `GET /galleries/:gallery_id/images` - List gallery-associated assets
- `DELETE /galleries/:gallery_id/images/:image_id` - Dissolve association

### Files & Scheduler Tasks
- `GET /files/:image_id` - Fetch the original physical asset stream
- `POST /refresh` - Dispatch background workers manually `{ "clear_orphans": true, "clear_temp": true, "refresh_hashes": false }`
