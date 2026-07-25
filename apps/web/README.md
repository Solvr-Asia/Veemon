# @veemon/web

React 19 + TanStack Router + TanStack Query, bundled by Vite and wrapped in a
Tauri v2 desktop shell. Talks to the Go API (`apps/api`) through the shared
`@veemon/api-client` (typed REST over the proto contract).

## Develop

```bash
bun install                 # from repo root
bun run --filter @veemon/web dev      # browser dev server on http://localhost:5173
```

Set the API base URL via `.env` (see `.env.example`); it defaults to
`http://localhost:3000`. For the login flow to work, add the Vite origin to the
Go server's `CORS_ORIGINS` (see `apps/api/.env.example`).

## Desktop (Tauri)

The desktop shell in `src-tauri/` requires a **Rust toolchain** (`rustup`,
`cargo`) plus your OS's webview dependencies — the browser dev flow above does
not. Once Rust is installed:

```bash
bun run --filter @veemon/web tauri:dev     # desktop window (spawns Vite)
bun run --filter @veemon/web tauri:build   # production desktop bundle
```

Generate the bundle icons once (writes `src-tauri/icons/`):

```bash
bun run --filter @veemon/web tauri icon path/to/logo.png
```

## Security notes

- **Token storage:** the auth token is kept in `localStorage` (`src/lib/auth.ts`)
  so the browser build works without server changes. This is readable by any
  script on the page — an XSS bug can steal it. For production, swap the storage
  seam for httpOnly cookies, in-memory + refresh, or the Tauri secure store.
- **CSP:** `src-tauri/tauri.conf.json` sets a restrictive `connect-src` that
  includes `http://localhost:3000`. Update it to match your real
  `VITE_API_BASE_URL` — do not widen it to `*`.

## Routes

File-based (`src/routes/`), compiled to a typed route tree
(`src/routeTree.gen.ts`) by `@tanstack/router-plugin`:

- `/` — home
- `/login` — email/password → `api.login()` → stores token → `/me`
- `/me` — `api.getMe()` + `api.listUsers()` (admin-only list)
