# AI Store Assistant — Design

**Date:** 2026-07-27
**Status:** Approved (design)
**Topic:** A conversational, read-only chat assistant over store operational data

---

## 1. Goal & Scope

This is **pass 2 of 3** in the Appsisten brainstorm series (`Database schema → AI features → Landing page`), following [`2026-07-27-database-schema-design.md`](2026-07-27-database-schema-design.md).

The brand "Appsisten" (App + Asisten) promises an assistant, but the only AI-flavored feature anywhere in the prototypes (`docs/prototypes/`) is a static, rule-based "Insight otomatis" card (canned text about revenue trend, funnel conversion vs. a benchmark, and stock-risk in Rupiah) — there is no real LLM behind it. The existing `apps/ai` service is a working LangGraph.js scaffold (one generic ReAct `assistant` agent with a `list_users` tool, one deterministic `user-report` workflow) but has no store-specific, product-facing feature yet.

**This pass designs:** a conversational chat assistant, scoped to a single store's operational data, that a store owner can ask natural-language questions like *"berapa omzet bulan ini?"* or *"produk apa yang stoknya menipis?"*.

**Confirmed via brainstorming Q&A:**
- Primary direction: a **conversational chat assistant**, not (yet) proactive automated insights or AI content generation — both are valuable but deferred (§5).
- **Read-only in v1** — no write/action tools. Lower risk, and avoids a prompt-injection-triggered mutation before trust is established (mirrors the security posture already documented on the existing `list_users` tool).
- Surfaced as a **chat panel inside the admin dashboard** (`apps/web`), not a WhatsApp integration (deferred, §5).
- **No fixed response language** — the assistant mirrors whatever language the owner writes in, rather than defaulting to Bahasa Indonesia.

### Non-goals (this pass)
- No write/action tools (create voucher, send broadcast, etc.).
- No WhatsApp or other non-dashboard channel.
- No proactive/scheduled insight generation.
- No content-generation features (product descriptions, marketing copy).
- No actual implementation — this is the design; an implementation plan follows via `writing-plans`.

---

## 2. Architecture Overview

```
apps/web (chat panel)
   │  authenticated PASETO request
   ▼
apps/api  ── new endpoint, fail-closed auth like every other route
   │  resolves + validates caller's active store membership
   │  passes store_id as a TRUSTED, server-set parameter (never client-supplied)
   ▼
apps/ai  ── existing HTTP surface, service-to-service (API_SERVICE_TOKEN)
   │  invokes the new `store_assistant` LangGraph agent
   │  agent calls read-only tools, scoped to the bound store_id
   ▼
apps/api's typed REST client (@veemon/api-client)
   │  same pattern the existing `list_users` tool already uses
   ▼
Postgres (via the schema from pass 1: orders, inventory, customers, products)
```

Response streams back up the same chain via SSE (reusing `apps/ai`'s existing `POST /agents/:name/stream` pattern).

**Why proxy through `apps/api` instead of `apps/web` calling `apps/ai` directly:** this repo's auth is fail-closed and centralized in `apps/api` (`RouteAuthConfig`/`mustAuthConfig`, per `codebase-conventions.md`). `apps/ai` today only ever authenticates *as a service* (`API_SERVICE_TOKEN`) calling *into* `apps/api` — it has no concept of an end-user session. Routing the browser request through `apps/api` first keeps a single auth perimeter and lets `apps/api` resolve the trusted `store_id` before `apps/ai` ever sees the request, rather than exposing `apps/ai` directly to the internet and trusting a client-supplied store identifier.

---

## 3. Key Design Decisions

1. **A new, dedicated agent — not an extension of the existing `assistant`.** The current `apps/application/agents/assistant.ts` is a generic, platform-level agent (its one tool, `list_users`, is about platform users, not store commerce data). The store assistant gets its own agent (`store_assistant`) with its own system prompt and tool set, so tenant-scoping rules and prompt design stay focused and auditable.
2. **Tenant scoping is enforced server-side, never trusted from the LLM.** Every tool call is bound to the `store_id` resolved by `apps/api` from the authenticated session — the LLM cannot be prompted (accidentally or via injection) into supplying a different store's ID and getting data back for it. This is the same principle already called out in the existing `list-users` tool's security-notes header.
3. **Read-only tool set for v1**, covering the pass-1 schema:
   - `get_sales_summary(period)` — omzet/orders/AOV/conversion for a date range (mirrors the dashboard KPI row).
   - `get_low_stock_products()` — inventory items at/below `reorder_point`.
   - `get_order_status(order_number)` — single order + its status timeline (`order_status_history`).
   - `list_recent_orders(status?)` — paginated order list, optionally filtered by status.
   - `get_customer_summary(phone_or_id)` — a customer's order count, spend, last-order date.
   - `get_top_products(period)` — best-selling products/variants by revenue or quantity.

   Each tool calls its corresponding `apps/api` REST endpoint via the existing typed client — no direct DB access from `apps/ai`, consistent with its current architecture. Each underlying endpoint needs its own explicit auth policy (fail-closed, per `codebase-conventions.md`) scoped to the caller's store.
4. **Conversation persistence splits across two systems, deliberately.** The existing Postgres-backed LangGraph checkpointer already durably stores full graph/message state per thread — that isn't duplicated. But checkpointer state isn't naturally listable in a UI (it's LangGraph's internal format), and the chat panel needs a "your past conversations" list. So this pass adds one small new table (§4) purely as addressable, listable metadata.
5. **No fixed response language.** The system prompt does not instruct a default language — the assistant mirrors the owner's input language, per the brainstorming decision.

---

## 4. New Schema (introduced by this pass)

| Table | Key fields | Notes |
|---|---|---|
| `chat_threads` | `id, store_id FK, user_id FK, title, last_message_at, created_at, updated_at, deleted_at` | Lives in `apps/api`'s Postgres (same DB as the pass-1 commerce schema), following the same conventions (UUID PK, soft delete). Message content/state is **not** duplicated here — it lives in `apps/ai`'s existing LangGraph checkpointer, addressed by this row's `id` as the thread ID. `title` can be auto-generated from the first message (implementation detail, not designed here). |

No other schema changes — all six tools read from the pass-1 commerce tables via existing/new `apps/api` query endpoints.

---

## 5. Deferred (Phase 2+)

- **Write/action tools** — e.g. "buat voucher diskon 10%" actually creating one. Would need an explicit propose-then-confirm UX (agent proposes, owner approves) before any mutation executes, given the read-only-first decision in this pass.
- **WhatsApp channel** — same agent/tools, different transport (WhatsApp Business API integration), fitting how Indonesian SME owners already operate (the prototype already assumes WhatsApp CRM broadcast).
- **Proactive/scheduled insights** — a workflow (not a chat-triggered agent run) that periodically calls the same read tools and pushes a summary, which could eventually replace the prototype's static "Insight otomatis" card with a real LLM-generated version. Deliberately not built first since the brainstorming decision prioritized the conversational surface.
- **AI content generation** — product descriptions, WhatsApp broadcast drafts, promo copy. A separate agent/tool set, unrelated to the read-only query tools here.
- **Cost/rate limiting** — LLM calls should likely be rate-limited per store (reusing the existing `middleware.RateLimitMiddleware` pattern on the new `apps/api` proxy endpoint) to bound cost; left as an implementation-plan detail, not a schema/architecture decision.

---

## 6. Dependencies & Open Questions Carried Forward

- **Depends on pass 1's flagged auth change:** this design assumes `apps/api` can resolve "the caller's currently active store" from `store_members` (see pass 1, §4.1 "Auth impact") — that auth cutover needs to land (or at least be planned) before this feature can be built.
- Exact REST endpoint shapes/paths for the six tools are implementation-plan detail, not designed here (per the proto-routes convention: declare via `veemon.route` options, then `make proto`).
- Same open question carried from pass 1: whether the repo/product renames from "Veemon" to "Appsisten."

---

## 7. Next Steps

1. Spec self-review (this doc).
2. User reviews this written spec.
3. **Pass 3 — Landing page:** brainstorm the copy/pricing/FAQ already drafted in the prototypes vs. what actually ships in `apps/web`, which currently has no landing page at all.
4. Each pass gets its own spec doc under `docs/superpowers/specs/`, then an implementation plan via the `writing-plans` skill.
