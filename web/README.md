# FlowForge console

A small Next.js App Router console for defining workflows, starting executions,
and inspecting step status and execution history. API calls run on the server;
the API address and optional bearer token are never exposed as browser settings.

Use Node.js 24:

```bash
npm ci
npm run dev
```

The default API URL is `http://localhost:8080`. Override `FLOWFORGE_API_URL` for
another environment. Set `FLOWFORGE_API_TOKEN` when the API uses `API_TOKEN`.
The development console opens at http://localhost:3000.

```bash
npm run lint
npm run typecheck
npm test
npm run build
```

The Docker image uses Next.js standalone output and a non-root runtime. From
the repository root, `docker compose --profile ui up --build -d --wait` starts it
with the API. The UI does not have user authentication: a configured API token
grants API access through the console to anyone who can reach it. Keep the
console on a trusted development network or protect it with an authenticated
gateway before exposing it.

The initial API has execution lookup by ID, without an execution listing
endpoint. The console reflects that boundary. Use **Refresh** on execution
detail to retrieve current state and history.

The lint configuration uses ESLint's compatibility utilities for the older
rule APIs in Next's bundled React/import plugins, allowing the current supported
ESLint version. TypeScript is pinned to the version supported by that parser.
