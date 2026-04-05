# Technology Stack

**Analysis Date:** 2026-04-04

## Languages

**Primary:**
- Go 1.24.0 - Used for main SDK implementation at `sdks/sandbox/go/`
- Python 3.10+ - SDK wrapper implementation at `sdks/sandbox/python/`
- TypeScript 5.7.2 - JavaScript/Node.js SDK at `sdks/sandbox/javascript/`
- JavaScript - Build and test scripts, API generation

**Secondary:**
- Bash - Makefile scripts and shell-based build tooling

## Runtime

**Environment:**
- Go 1.24.0 - Runtime for Go SDK
- Python 3.10, 3.11, 3.12, 3.13 - Runtime for Python SDK
- Node.js 20+ - Runtime for TypeScript/JavaScript SDK
- Browser runtime - JavaScript SDK supports browser environments

**Package Manager:**
- Go: Built-in `go` package manager (no separate tool)
  - Lockfile: `go.sum` present
- Python: `uv` (Python package installer)
  - Lockfile: `pyproject.toml` with dependency groups
- JavaScript: `pnpm` 9.15.0
  - Lockfile: `pnpm-lock.yaml` (implied)

## Frameworks

**Core:**
- OpenAPI code generators - Generated API clients for Lifecycle, Execd, and Egress APIs
  - Go: `oapi-codegen` v1.3.1 runtime
  - Python: `openapi-python-client` 0.28.0+ (dev)
  - JavaScript: `openapi-typescript` 7.9.1 (dev)

**Testing:**
- Go: Built-in `testing` package
- Python: `pytest` 7.0.0+
  - `pytest-asyncio` 0.21.0+ - Async test support
  - `pytest-cov` 4.0.0+ - Coverage reporting
- JavaScript: Node.js built-in `node --test` (no external framework)

**Build/Dev:**
- Go: `Make` (Makefile at `sdks/sandbox/go/Makefile`)
- Python: `uv` package manager with Make
  - `black` - Code formatter
  - `isort` - Import sorting
  - `ruff` 0.14.8+ - Linter
  - `pyright` 1.1.0+ - Type checker
  - `pytest-cov` - Coverage
- JavaScript:
  - `tsup` 8.5.0 - TypeScript bundler/builder
  - `eslint` 9.39.2 - Linter
  - `typescript` 5.7.2 - Type checker and transpiler

## Key Dependencies

**Critical:**
- `github.com/google/uuid` v1.6.0 (Go) - UUID generation for sandbox IDs
- `pydantic` 2.4.2+ (Python) - Data validation and configuration management
- `httpx` 0.27.0+ (Python) - Async HTTP client for API requests
- `openapi-fetch` 0.14.1 (JavaScript) - Lightweight OpenAPI client
- `undici` 7.18.2 (JavaScript) - HTTP client for Node.js with connection pooling

**Infrastructure:**
- `github.com/oapi-codegen/runtime` v1.3.1 (Go) - OpenAPI code generation runtime
- `python-dateutil` 2.8.2+ (Python) - Date/time utilities
- `attrs` 21.3.0+ (Python) - Class utilities for generated API code
- `typescript-eslint` 8.52.0 (JavaScript) - TypeScript linting

## Configuration

**Environment:**
- Connection configuration via environment variables:
  - `OPEN_SANDBOX_DOMAIN` - Server address (default: `localhost:8080`)
  - `OPEN_SANDBOX_API_KEY` - API authentication token
  - `OPEN_SANDBOX_PROTOCOL` - Protocol (http/https, default: http)
- Configuration objects in all SDKs:
  - Go: `ConnectionConfig` struct at `sdks/sandbox/go/opensandbox/config.go`
  - Python: `ConnectionConfig` Pydantic model at `sdks/sandbox/python/src/opensandbox/config/connection.py`
  - JavaScript: `ConnectionConfig` class at `sdks/sandbox/javascript/src/config/connection.ts`

**Build:**
- Go:
  - `Makefile` at `sdks/sandbox/go/Makefile` with targets: `generate`, `build`, `vet`, `test`, `test-integration`, `test-staging`, `lint`
  - oapi-codegen config: `cfg.yaml` files in API spec directories
- Python:
  - `pyproject.toml` with build system `hatchling` + `hatch-vcs`
  - Tool configs: `[tool.ruff]`, `[tool.pyright]`, `[tool.pytest.ini_options]`, `[tool.coverage.run]`
  - Makefile targets for local development
- JavaScript:
  - `tsconfig.json` at `sdks/sandbox/javascript/tsconfig.json` extends base config
  - `tsup.config.ts` for multi-format building (ESM + CJS)
  - `eslint.config.mjs` with TypeScript support
  - `package.json` build script: `pnpm run gen:api && tsup`

## Platform Requirements

**Development:**
- Go SDK: Go 1.24.0, Make, oapi-codegen
- Python SDK: Python 3.10+, uv, Make
- JavaScript SDK: Node.js 20+, pnpm 9.15.0+

**Production:**
- OpenSandbox API server (accessible via configured domain/protocol)
- HTTP/1.1 or HTTP/2 support

---

*Stack analysis: 2026-04-04*
