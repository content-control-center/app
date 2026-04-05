# Content Control Center Application

## Backend development

TBD

---

## Frontend development

### API documentation

The full REST API reference is available at **[https://api.contentcontrol.center](https://api.contentcontrol.center)**.

---

### Prerequisites

| Tool | Minimum version | Notes |
|------|----------------|-------|
| [Docker Desktop](https://www.docker.com/products/docker-desktop/) | 4.x | Includes Compose v2 |
| [Node.js](https://nodejs.org/) | 20 LTS | Only needed for editor tooling / linting outside Docker |

You do **not** need Go, a local database, or any other backend tooling — the API runs inside Docker.

---

### Development setup

#### 1. Clone the repository

```bash
git clone https://github.com/content-control-center/app.git
cd app
```

#### 2. Pull the latest backend image

```bash
docker compose pull
```

This downloads the pre-built Go API image from Docker Hub. You only need to repeat this when a new backend release is available.

#### 3. Start the development environment

```bash
docker compose up
```

Open **http://localhost:5173** in your browser. The app will hot-reload automatically every time you save a file under `web/src/`.

> **First start** takes a moment while `npm install` runs inside the container. Subsequent starts are fast because `node_modules` is cached in the Docker volume.

#### 4. Stop the environment

```bash
# Stop containers (database is preserved)
docker compose down

# Stop containers AND wipe the database volume
docker compose down -v
```

---

### Daily workflow

```bash
# Pull latest API image before starting work
docker compose pull && docker compose up
```

Edit any file under `web/src/` — the browser updates instantly via Hot Module Replacement (HMR). No container restarts are required.

---

### Environment variables

Vite environment variables must be prefixed with `VITE_` to be accessible in the browser.

Create a `web/.env.local` file (gitignored) to override defaults:

```dotenv
# Example: point to a staging API instead of the local container
VITE_API_BASE_URL=https://staging.api.contentcontrol.center
```

---

### Updating the backend

When a new version of the API is released, pull the updated image and restart:

```bash
docker compose pull
docker compose up
```

Your database data is stored in a named Docker volume (`db-data`) and is not affected by image updates.