# Database Schema — Design

**Date:** 2026-07-27
**Status:** Approved (design)
**Topic:** Detailed, phase-tagged relational schema for the "Appsisten" storefront-migration SaaS

---

## 1. Goal & Scope

`docs/prototypes/` (local, untracked design-comp exports) documents the real product
this monorepo is being built into: **Appsisten** (formerly "Pindah") — a SaaS letting
Indonesian marketplace sellers (Shopee/Tokopedia/TikTok Shop) one-click-import their
catalog and run their own zero-commission storefront + back-office (orders, inventory,
CRM, marketing, reports, team). The current Go backend (`apps/api`) is a near-blank
slate: one `users` table, no tenant table, no AI schema.

This is **pass 1 of 3** sequential brainstorm→spec passes for the product
(`Database schema → AI features → Landing page`). Each pass gets its own design doc
and, later, its own implementation plan.

**This pass's scope:** design the **full** relational schema implied by the prototype's
feature set, phase-tagged (**Phase 1** = MVP, buildable now; **Phase 2+** = designed but
not scheduled), rather than only a lean MVP subset — the prototype mockups already
specify these entities in detail, so capturing them now avoids losing that design work.

### Non-goals (this pass)
- No AI/agent/embedding schema — that's pass 2.
- No landing-page/marketing content modeling — that's pass 3.
- No actual migration files or Go entity code — this is the schema *design*; implementation is a separate plan.
- No multi-warehouse inventory or per-store custom RBAC (see §6, deliberately deferred).

---

## 2. Conventions

All new tables follow this repo's established conventions
([`codebase-conventions`](../../../.claude/rules/codebase-conventions.md),
[`database`](../../../.claude/rules/database.md)):

- UUID primary keys, generated in a `BeforeCreate` GORM hook (matching `entity.User`) or via `gen_random_uuid()` default at the SQL level.
- Soft delete via `deleted_at` + a partial unique index (`WHERE deleted_at IS NULL`) wherever a natural-key uniqueness constraint exists (e.g. store slug, SKU).
- `created_at`/`updated_at` columns, reusing the existing `update_updated_at_column()` trigger function.
- golang-migrate numbered SQL files, continuing from `000002` (current latest is `000001_create_users_table`).
- One `entity/<name>.go` per aggregate; one `repository/<name>_repository/` package per aggregate exposing `ListParams`-style pagination (matching `user_repository`).
- Money stored as `bigint` (Indonesian Rupiah has no practical subunit) — no multi-currency support, matching the Indonesia-only market shown throughout the prototypes.

---

## 3. Key Design Decisions

These were the load-bearing decisions made during brainstorming (see rationale inline):

1. **Tenancy root is a `stores` table, not a company string.** Today, tenancy is a loose `company_code` varchar on `users` with no FK and no dedicated table. The product's real tenant unit is a *store* (e.g. "Senja Skin"), so `stores` becomes the tenant root.
2. **Store↔User is many-to-many via `store_members`.** One login can own or staff multiple stores (matches the invite-by-email "Tim & Akses" flow, and is the more realistic long-term model over a rigid one-user-one-store FK).
3. **Two separate role concepts.** `users.roles` stays platform-level only (internal Appsisten staff, e.g. `superadmin`/support) and is unrelated to `store_members.role`, which carries the store team role (Pemilik/Admin/Staf Gudang/Kasir-CS/Finance/Marketing) shown in the prototype's Team & Access screen. The role→permission matrix (Produk&Katalog / Pesanan&Pengiriman / Pelanggan / Pembayaran&Keuangan / Broadcast&Promo / Tim&Akses × full/edit/view/none) is a **fixed mapping in application code**, not a database table — the prototype shows it as fixed per role, not store-customizable.
4. **Product variants use a full option matrix**, not a single free-text variant label. The prototype's import-mapping screen shows one free-text "Nama Varian 1" field, but a normalized `product_options` → `product_option_values` → `product_variants` (+ join table) model is used instead, because the core wedge feature is importing catalogs from Shopee/Tokopedia/TikTok, all of which model variants this way (e.g. Warna × Ukuran).

---

## 4. Phase 1 (MVP) Schema

### 4.1 Identity & Tenancy

| Table | Key fields | Notes |
|---|---|---|
| `users` (existing, modified) | `id, email, password, name, phone, status, roles (platform-level only), created_at, updated_at, deleted_at` | **Drop `company_code`** — tenancy moves entirely to `store_members`. |
| `stores` | `id, name, slug (unique, e.g. "senjaskin" → senjaskin.appsisten.id), custom_domain (nullable, unique), status (onboarding/active/suspended), timezone, currency (default IDR), created_at, updated_at, deleted_at` | The tenant root — one row per seller's shop. |
| `store_members` | `id, store_id FK, user_id FK, role (owner/admin/warehouse_staff/cashier_cs/finance/marketing), status (invited/active), invited_email, invited_at, joined_at, created_at, updated_at, deleted_at` | Unique `(store_id, user_id)` where not deleted. |
| `store_subscriptions` | `id, store_id FK, plan (mulai/bisnis/skala), billing_cycle (monthly/yearly), status (trialing/active/past_due/canceled), current_period_ends_at, created_at, updated_at` | Backs the 3-tier pricing (Mulai free / Bisnis Rp149rb / Skala Rp399rb) surfaced on the landing page (pass 3). |

**Auth impact (flagged for the implementation plan, not designed here):** PASETO claims (`pkg/token`) and `middleware.AuthContext` currently carry `CompanyCode`. This must change to carry the authenticated user's store membership(s) — likely with an "active store" concept scoped per session/request — once `store_members` replaces `company_code`.

### 4.2 Catalog

| Table | Key fields | Notes |
|---|---|---|
| `categories` | `id, store_id FK, parent_id (self FK, nullable), name, slug, icon, sort_order, is_visible, created_at, updated_at, deleted_at` | Hierarchical (parent/sub-category), per-store. |
| `products` | `id, store_id FK, category_id FK (nullable), name, slug, brand, description, weight_grams, length_cm, width_cm, height_cm, status (active/draft/archived), source (manual/shopee_import/tokopedia_import/tiktok_import), source_external_id (nullable), created_at, updated_at, deleted_at` | `source_external_id` enables re-sync/dedup against the originating marketplace listing. |
| `product_images` | `id, product_id FK, url, sort_order, created_at` | |
| `product_options` | `id, product_id FK, name (e.g. "Warna"), sort_order` | |
| `product_option_values` | `id, product_option_id FK, value (e.g. "Merah"), sort_order` | |
| `product_variants` | `id, product_id FK, sku (unique per store), barcode (nullable), price, weight_override_grams (nullable), status, created_at, updated_at, deleted_at` | One row per SKU (per option combination). |
| `product_variant_option_values` | `(product_variant_id, product_option_value_id)` composite PK | Join table: which option values compose this variant (e.g. Merah + L). |
| `product_price_tiers` | `id, product_variant_id FK, min_qty, price, sort_order` | Wholesale/tiered pricing — flagged "planned/unmapped" in the prototype's import-mapping screen; modeled properly here rather than deferred. |

### 4.3 Inventory

| Table | Key fields | Notes |
|---|---|---|
| `inventory_items` | `id, store_id FK, product_variant_id FK (unique), on_hand, reserved, reorder_point, cost_price, updated_at` | `available` (on_hand − reserved) and `level` (Aman/Rendah/Habis) are computed at read time, not stored. Single-location for now (see §6). |
| `stock_ledger_entries` | `id, store_id FK, inventory_item_id FK, type (in/out/adjustment), quantity, reference_type (order/purchase/adjustment/opname), reference_label, performed_by_user_id FK, balance_after, created_at` | Append-only audit log (no update/soft-delete) — matches the prototype's "Kartu Stok". |
| `stock_opnames` | `id, store_id FK, opname_date, status (draft/completed), created_by_user_id FK, created_at, completed_at` | Physical stock count sessions. |
| `stock_opname_items` | `id, stock_opname_id FK, inventory_item_id FK, counted_qty, system_qty, diff, note` | |

### 4.4 Orders & Customers

| Table | Key fields | Notes |
|---|---|---|
| `customers` | `id, store_id FK, name, phone (unique per store), email (nullable), first_order_at, last_order_at, orders_count (cached), total_spent (cached), created_at, updated_at, deleted_at` | Segment (Sering beli / Pelanggan baru / Lama tidak belanja) is computed from these fields, not stored. |
| `orders` | `id, store_id FK, order_number (e.g. INV-2042, unique per store), customer_id FK (nullable — guest checkout), channel (storefront/shopee/tokopedia/tiktok/whatsapp/funnel), status (new/processing/shipped/completed/cancelled), payment_status (unpaid/paid/cod), payment_method, subtotal, discount_total, shipping_cost, total, shipping_recipient_name, shipping_phone, shipping_address_line, shipping_city, shipping_province, shipping_postal_code, courier, tracking_number, voucher_id (nullable FK, Phase 2), notes, created_at, updated_at, deleted_at` | |
| `order_items` | `id, order_id FK, product_variant_id FK, product_name_snapshot, variant_label_snapshot, sku_snapshot, unit_price, quantity, subtotal, created_at` | Snapshot fields protect order history if a product/variant is later renamed or deleted. |
| `order_status_history` | `id, order_id FK, from_status, to_status, note, changed_by_user_id (nullable = system), created_at` | Powers the order-timeline UI (Dibuat → Dibayar → Diproses → Dikirim → Selesai). |

### 4.5 Marketplace Import (the core wedge feature)

| Table | Key fields | Notes |
|---|---|---|
| `marketplace_connections` | `id, store_id FK, platform (shopee/tokopedia/tiktok_shop), external_shop_ref, status (connected/disconnected/error), connected_at, last_synced_at, created_at, updated_at` | Credentials/tokens are referenced via a secrets store — never stored raw in this table (OWASP A02). |
| `import_jobs` | `id, store_id FK, marketplace_connection_id FK (nullable if CSV upload), source_type (marketplace_link/csv_upload), status (pending/mapping/importing/completed/failed), total_items, matched_items, review_items, unmapped_items, started_at, completed_at, created_at` | |
| `import_job_items` | `id, import_job_id FK, external_product_id, raw_payload (JSONB), mapping_status (matched/review/unmapped/default), mapped_product_id (nullable FK), created_at` | `raw_payload` is intentionally JSONB — the one place schema-less storage fits, since marketplace payload shapes vary and are read once during mapping review, never queried structurally. |

---

## 5. Phase 2+ Schema (designed now, scheduled later)

Kept at lighter detail — full column lists will be finalized when each is actually scheduled for implementation.

- **Promo engine:** `vouchers` (name, code, discount_type: percent/nominal/free_shipping/buy_x_get_y, discount_value, max_discount_cap, min_purchase rules, applies_to scope, quota_total, quota_per_customer, first_order_only, stackable, channels, target_segment, start/end dates, budget_cap, auto_apply, status) + `voucher_applicable_categories`/`voucher_applicable_products` (scoping) + `voucher_redemptions` (usage tracked against quota).
- **Funnel builder:** `funnels` + `funnel_steps` (landing/offer/order_form/upsell/checkout/thank_you, JSONB `config` for page content) + `funnel_step_events` (view/convert, aggregated for conversion analytics) + `order_form_fields` (configurable custom checkout fields).
- **Reseller program:** `resellers` (linked to `customers`, referral_code, tier, status) + `reseller_tiers` (store-configurable commission %, min monthly sales — prototype defaults Bronze 8%/Silver 12%/Gold 15%) + `reseller_referral_orders` (commission per order).
- **Custom audiences (ads retargeting):** `audiences` (name, sync_meta, sync_tiktok) + `audience_rules` (field/operator/value).
- **Bookkeeping:** `expenses` (date, category: ads/stock/operational/salary/subscription, description, amount, payment_method).
- **Store presentation/config:** `store_storefront_settings` (1:1 with store — theme_template, primary_color, hero copy/image, section toggles) + `store_payment_methods` (qris/bank_transfer/cod/credit_card, provider e.g. Xendit) + `store_couriers` (jne/jnt/sicepat/anteraja, enabled flag).
- **CRM pipeline:** `crm_leads` (customer_id nullable, stage: new/follow_up/negotiation/closing/loyal, value, channel, last_contacted_at, note).

---

## 6. Deliberately Deferred (YAGNI)

- **Multi-location/warehouse inventory** — the prototype only shows a single "Staf Gudang" (warehouse staff) role and no multi-location UI. If needed later: add a `store_locations` table and a `location_id` FK on `inventory_items`.
- **Per-store custom RBAC** — the role→permission matrix is fixed in the prototype (not store-configurable). A `store_role_permissions` override table can be added later if stores need custom roles.

---

## 7. Migration & Rollout Notes

This is not a greenfield change — `users.company_code` is live today and threaded through `pkg/token`, `pkg/middleware`, and `config/bootstrap.go`. Rollout sequence for the implementation plan:

1. Create `stores` and `store_members` via new migrations.
2. Backfill: one `stores` row per distinct existing `company_code` value; `store_members` rows linking existing users, with `role` derived from their current `roles` array.
3. Cut over auth (PASETO claims + `AuthContext`) to use store membership instead of `company_code`.
4. Drop the `company_code` column in a later migration once the cutover is verified.

Files follow the existing per-aggregate pattern: `apps/api/entity/store.go`, `apps/api/migrations/000002_create_stores_table.up/down.sql`, `apps/api/repository/store_repository/`, and so on — one migration pair and one entity file per table above, added incrementally per implementation phase.

---

## 8. Open Question Carried Forward

Whether the repo/product itself gets renamed from "Veemon" to "Appsisten" (or "Veemon" stays as an internal codename) is a branding decision outside this doc's scope. Flagging so it isn't lost before the AI (pass 2) and Landing page (pass 3) design passes.

---

## 9. Next Steps

1. Spec self-review (this doc) — placeholder/consistency/scope/ambiguity check.
2. User reviews this written spec.
3. **Pass 2 — AI features:** brainstorm what "Appsisten" the assistant should actually do beyond the prototype's mocked "Insight otomatis" card (real LLM-backed insights, a chat assistant, etc.), informed by this schema and the existing minimal `apps/ai` LangGraph service.
4. **Pass 3 — Landing page:** brainstorm the copy/pricing/FAQ already drafted in the prototypes vs. what actually ships in `apps/web`, which currently has no landing page at all.
5. Each pass gets its own spec doc under `docs/superpowers/specs/`, then an implementation plan via the `writing-plans` skill.
