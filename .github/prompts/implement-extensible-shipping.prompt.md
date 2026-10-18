---
description: "Continue the EUROMNI extensible shipping model and implement a bounded, tested transport milestone"
name: "EUROMNI — Transport extensible"
argument-hint: "Optional: specify the shipping milestone or provider/service to focus on"
agent: "agent"
---

Continue EUROMNI's transport domain from the current repository state. Read
[`docs/transport-extensible.md`](../../docs/transport-extensible.md),
[`docs/marches-localisation-europe.md`](../../docs/marches-localisation-europe.md),
[`docs/architecture.md`](../../docs/architecture.md),
[`docs/database.md`](../../docs/database.md), and the current shipping API,
store, checkout, order schema, admin UI, and PostgreSQL/MariaDB integration
tests before editing.

## Verified restart baseline (18 October 2026)

- Current `main` and `origin/main` point to `0b9c7a6` (`Separate shipping
  services and rate versions`); the worktree was clean at this checkpoint.
- Services are distinct from carriers; legacy carrier-only requests map to the
  `legacy` service. Admin rate saves create UUID revisions and append immutable
  tariff/rule snapshots. Orders store service code/name and the selected
  revision.
- The next bounded task is an admin history view by carrier/service/zone,
  comparison of versions, and non-destructive restore. Restore by copying an
  old definition into current config and saving it as a new version; never edit
  historical rows. Preserve order-to-version references.
- Do not add carrier API calls, labels, packaging, delivery estimates or new
  pricing semantics in this task. Ask before expanding into those decisions.
- Verified on the checkpoint: full Go suite (233 tests), PostgreSQL store/API
  integrations, MariaDB 11.8.2 `TestMariaDBMigrationAndCommerceFlow`, and
  `node --check` for admin/storefront scripts. The portable MariaDB server is
  stopped; its user-profile install and dedicated datadir remain available.

Before editing, inspect the current history schema, admin authorization pattern,
config replacement transaction and order snapshot readers. Add route validation,
authorization, PostgreSQL/MariaDB integration coverage, and update the transport
docs. Do not commit or push unless asked.

## Product direction

Build native, provider-neutral shipping capabilities that can later be refined
by trusted EUROMNI métier modules compiled to Wasm. Do not hard-code La Poste,
Colissimo, Chronopost, DPD, or Selfnamed rules as closed enums. The current
carrier/zone/fixed-rate path is production-shaped groundwork and must keep
working during additive migration.

## Required design boundaries

- Keep carrier, selectable service, destination/zone, shipping origin, physical
  parcel/product attributes, rating rule, quote, shipment execution, and order
  snapshot as distinct concepts.
- Support exact decimal prices and explicit ISO currencies. Do not infer price
  conversion from country metadata, language, or a carrier's name.
- Design native tariff rules for fixed price, weight bands, cart-value bands,
  and explicitly configured per-item/per-parcel charges. Boundaries, priority,
  combination, validity and no-match behavior must be deterministic.
- Preserve preparation and transit estimates separately. Label estimates as
  estimates unless a contractual source guarantees them.
- A selected quote must be revalidated at checkout. On stale, expired,
  unavailable or invalid quote, fail explicitly; never silently replace it
  with another service or rate.
- Store an immutable order quote snapshot: selected carrier/service, origin and
  destination context, price/currency breakdown, rule or module/build version,
  quote validity/source, physical calculation inputs and delivery estimate.
- Keep the Wasm contract typed, bounded, versioned and capability-gated. Guests
  never access SQL, filesystem, network or secrets. Validate all guest output
  in Go. Any carrier API call remains a host-side adapter with explicit
  allowlisting, timeouts and quotas.
- Keep taxes separate from shipping calculations; delivery amount must not
  pretend to include unimplemented VAT.
- Keep PostgreSQL and MariaDB schemas/adapters/tests aligned. Do not claim
  MariaDB runtime validation unless the dedicated integration test ran against
  a real configured test database.

## Working method

1. Inspect the actual worktree first; preserve unrelated edits and do not
   overwrite uncommitted work.
2. Compare the requested change with the current schema and APIs. If the next
   milestone is ambiguous, implement only the smallest safe vertical slice
   consistent with the documented phases; ask before decisions that change
   pricing semantics, package allocation, provider contracts, or legal promises.
3. Prefer additive migrations and explicit backward compatibility. Convert
   existing fixed rates without loss and preserve old order snapshots.
4. Add targeted unit/API/store tests, including boundary values, stale quotes,
   failure paths, and authorization. Update directly related API, database and
   storefront documentation.
5. Run formatting, focused tests, `go test ./...`, integration-test compilation
   and PostgreSQL integrations when available. Run MariaDB integration only
   against a dedicated configured test database.
6. Report changed behavior, tests actually executed, unresolved provider/legal
   decisions, and whether the worktree remains uncommitted. Do not commit or
   push unless requested.

If a focus was supplied in the prompt argument, apply it only if it remains
consistent with this model and the repository. Otherwise, begin with native
versioned services and tariff rules, not carrier API integrations.
