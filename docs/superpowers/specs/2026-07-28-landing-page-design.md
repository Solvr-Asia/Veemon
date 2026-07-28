# Landing Page — Design

**Date:** 2026-07-28
**Status:** Approved (design)
**Topic:** A new static marketing site (`apps/landing-page`) for Appsisten

---

## 1. Goal & Scope

This is **pass 3 of 3** in the Appsisten brainstorm series, following
[`2026-07-27-database-schema-design.md`](2026-07-27-database-schema-design.md) and
[`2026-07-27-ai-store-assistant-design.md`](2026-07-27-ai-store-assistant-design.md).

`apps/web` today has **no landing page** — it's architected as a Tauri-wrapped
desktop app shell (`/`, `/login`, `/me`), not a public marketing site. The
prototypes (`docs/prototypes/`) already contain a fully drafted landing page —
`Appsisten Landing.dc.html` — with hero, features, pricing, and FAQ copy in Bahasa
Indonesia, plus a warmer olive/terracotta editorial visual direction (an animated
WebGL fluid-noise hero background) distinct from the flatter "Pindah"-branded
version.

**This pass designs:** a new, dedicated static marketing site adapting that
Appsisten-branded content and visual direction.

**Confirmed via brainstorming Q&A:**
- Lives in a **new app, `apps/landing-page`** — not new routes bolted onto `apps/web` — since `apps/web` is Tauri-first and not a natural fit for a public, SEO-facing site.
- Adapts the **Appsisten** copy and visual direction (not "Pindah", not fresh copy) — matches the wordmark exploration's chosen direction and the more premium WebGL-hero treatment.
- Built with **Astro** — static-site generation for strong SEO/first-paint, with React used only for interactive "islands" (the pricing toggle, the WebGL hero canvas).
- **Static only, no backend integration** — CTAs link out to `apps/web` for signup/login; no embedded form calling `apps/api`.

### Non-goals (this pass)
- No embedded signup/waitlist form or `apps/api` integration.
- No blog, docs, or additional content pages beyond the single marketing page.
- No i18n/English translation.
- No actual implementation — this is the design; an implementation plan follows via `writing-plans`.

---

## 2. Architecture

- **New Bun workspace member:** `apps/landing-page`, added to the root workspace list and root `dev`/`build` scripts, following the same monorepo conventions as `apps/web`/`apps/ai` (per `project-overview.md`).
- **Astro**, statically generated (SSG) — every page pre-rendered at build time, deployable as static assets to any CDN/static host. No server runtime, and no dependency on `apps/api` or `apps/ai` at request time — fully decoupled, matching the "static only" decision.
- **React islands** (Astro's islands architecture — hydrated only where needed, zero JS elsewhere):
  - Pricing monthly/yearly toggle.
  - The WebGL fluid-noise hero background (adapted from the custom GLSL already written in `Appsisten Landing.dc.html`) — the one genuinely animated, canvas-driven piece.
  - (FAQ accordion can likely be plain Astro/CSS `<details>` — no React needed.)
- **CTAs link out**, not in-app: primary "Coba gratis" and secondary "Masuk" point to `apps/web`'s signup/login routes. Exact target (e.g. a subdomain like `app.appsisten.id` vs. a relative path under a shared domain) is a deployment-config detail for the implementation plan, not fixed here.

---

## 3. Page Structure

Single page with anchor-based nav (matches the prototype's convention — `Fitur` / `Cara Kerja` / `Harga` / `FAQ` all scroll-link within one page, not separate routes). Sections, adapted from `Appsisten Landing.dc.html`:

1. **Nav** — Fitur / Cara Kerja / Harga / FAQ, "Masuk" / "Coba gratis" (links out to `apps/web`).
2. **Hero** — headline ("Pindahkan tokomu dari marketplace ke toko sendiri"), animated WebGL background, stat bar, TikTok Shop import callout, a cost-savings example callout.
3. **Marketplace marquee** — Shopee / Tokopedia / TikTok Shop / Lazada / Instagram logos, scrolling.
4. **Features grid** (6 items) — Impor otomatis, Storefront sendiri, Database pelanggan, Inventaris & stok, Voucher & promo, Laporan cerdas.
5. **Cara Kerja** (3 steps) — Hubungkan sumber → Impor otomatis → Toko aktif.
6. **Storefront bullets** — Tema siap pakai, Editor visual, Mobile-first, Domain sendiri.
7. **Pricing** (3 tiers, monthly/yearly toggle) — **Mulai / Bisnis / Skala**, matching the `store_subscriptions.plan` enum from the pass-1 schema so marketing copy and billing data stay in sync.
8. **Testimonial** — Dewi Anggraini, Senja Skin.
9. **FAQ** (5 Q&A) — import data safety, no commission, can still sell on marketplaces in parallel, migration speed, custom domain availability.
10. **Footer** — Produk / Perusahaan / Bantuan columns.

---

## 4. Content Caveat — Fictional Social Proof

The prototype's hero/social-proof copy includes specific numbers that are **mockup
data, not real**: "2.400+ penjual sudah pindah", "4,9★ rating penjual", and the
"Rp50jt/bln → hemat ±Rp3jt" cost-savings example. These read as concrete, checkable
claims. **Before actual launch, these must be replaced with real figures, backed by
generic/qualitative framing (e.g. "Ratusan penjual telah pindah"), or removed** —
shipping fabricated statistics as real social proof is a legal/trust risk, not just
a copy-polish detail. This is a launch-readiness blocker, tracked here so it isn't
lost by the time this becomes an implementation plan.

---

## 5. Deferred (Phase 2+)

- Embedded signup/waitlist form with direct `apps/api` integration (would need `@veemon/api-client` wired into a previously-static app).
- Additional pages: blog, docs, about, careers (referenced in the footer's "Perusahaan" column but not built now).
- i18n/English version.
- CMS-driven copy (currently hardcoded Astro content) if non-engineers need to edit marketing copy independently.

---

## 6. Dependencies & Open Questions Carried Forward

- Domain/hosting split between `apps/landing-page` (public marketing) and `apps/web` (authenticated app) — e.g. root domain vs. `app.` subdomain — is a deployment decision for the implementation plan, not fixed here.
- The fictional social-proof numbers (§4) must be resolved with real data (or removed) before launch.
- Same open question carried from passes 1 and 2: whether the repo/product renames from "Veemon" to "Appsisten."

---

## 7. Next Steps

1. Spec self-review (this doc).
2. User reviews this written spec.
3. All three passes (schema, AI assistant, landing page) are now spec'd. Move to implementation planning via the `writing-plans` skill — likely starting with the pass-1 schema/tenancy migration, since both the AI assistant (pass 2) and the landing page's plan/pricing sync (pass 3) depend on it.
