# @veemon/ai

A [LangGraph.js](https://langchain-ai.github.io/langgraphjs/) service. Its agents
and workflows reach the Go API (`apps/api`) through the shared `@veemon/api-client`,
so the AI inherits the Go server's auth policy and validation — one API surface,
three consumers. The graphs are served over a small [Hono](https://hono.dev)
HTTP API (invoke + SSE stream), and thread state is persisted by a LangGraph
checkpointer (in-memory or Postgres), which enables durable multi-turn memory,
resumable runs, and human-in-the-loop interrupts.

## Architecture (clean architecture)

Dependencies point inward: `application` depends on `domain` ports; `infrastructure`
implements them; the composition root wires them together and the HTTP layer only
sees a small `Graph` interface.

```
src/
  domain/                    ports & types (no framework deps)
    users.ts                 UserDirectory port + UserSummary
  application/               the graphs (framework-facing, but backend-agnostic)
    tools/list-users.ts      list_users tool (uses the UserDirectory port)
    agents/assistant.ts      tool-calling ReAct agent (createReactAgent)
    workflows/user-report.ts explicit StateGraph: fetch users -> report count
    graph.ts                 the Graph interface the HTTP layer consumes
  infrastructure/            adapters implementing the ports
    api/                     @veemon/api-client + UserDirectory adapter
    llm/model.ts             ChatAnthropic factory
    checkpointer/            MemorySaver | PostgresSaver factory
  interface/http/server.ts   Hono routes (invoke + SSE stream)
  config.ts                  env config (validated, fail-fast)
  composition.ts             composition root — registers config/adapters/graphs
                             into an Awilix container, resolved via `.cradle`
  main.ts                    entry point (Bun serves the default export)
```

## Run

```bash
bun install                          # from repo root
bun run build:client                 # build @veemon/api-client types (once)
bun run --filter @veemon/ai dev        # http://localhost:4111 (watch mode)
```

Running the **agents** requires an Anthropic API key (`.env`, see `.env.example`);
the server, `/health`, and the **workflows** boot and run without one (the model
is constructed lazily, so a missing key surfaces only when an agent is invoked).
Building and testing never need a key. Configure the model with `AI_MODEL`
(default `claude-opus-4-8`) / `AI_MAX_TOKENS`, the target API with `API_BASE_URL` /
`API_SERVICE_TOKEN`, and persistence with `CHECKPOINTER` (`memory` | `postgres`,
the latter reusing `DATABASE_URL`).

## HTTP API

```
GET  /health                        liveness + registered agents/workflows
POST /agents/:name/invoke           { message, thread_id? } -> { thread_id, result }
POST /agents/:name/stream           { message, thread_id? } -> SSE (start/update/done/error)
POST /workflows/:name/invoke        { input, thread_id? }   -> { thread_id, result }
POST /workflows/:name/stream        { input, thread_id? }   -> SSE
```

Pass a stable `thread_id` to continue a conversation/run against its checkpointed
history; omit it and one is generated per request.

```bash
# Workflow (no model/API key needed; needs the Go API up)
curl -s -X POST localhost:4111/workflows/user-report/invoke \
  -H 'content-type: application/json' -d '{"input":{"page":1,"size":10}}'

# Agent (needs ANTHROPIC_API_KEY)
curl -s -X POST localhost:4111/agents/assistant/invoke \
  -H 'content-type: application/json' -d '{"message":"How many users are registered?"}'
```

## Security notes

The agent's tools call the API with the **service** identity (`API_SERVICE_TOKEN`),
not the end user's. If the agent is exposed to untrusted input, prompt injection
can make it call a tool and pull data (including PII) into the model context. The
LLM is not an authorization boundary. Before going beyond a demo: scope
`API_SERVICE_TOKEN` to least privilege (never reuse an admin token), keep
authorization on the API side, prefer aggregates over raw PII, and add guardrails
/ approval for sensitive tools (LangGraph interrupts make approval gates easy).
See the header of `src/application/tools/list-users.ts`.

## Test / typecheck

```bash
bun run --filter @veemon/ai test       # tool logic against a fake UserDirectory
bun run --filter @veemon/ai typecheck  # tsc --noEmit
```
