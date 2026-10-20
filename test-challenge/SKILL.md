---
name: test-challenge
description: Challenge whether tests deserve to exist — KEEP / DELETE / CONVERT / MISSING per test, by Khorikov's quadrants and transport/infra/business layer rules. Use before writing any tests, including ones you are about to add for code you just wrote when nobody asked; when reviewing tests or a PR/diff that adds them; on "should I test this"; on complaints about too many, brittle, mock-heavy or useless tests; and for test cleanup or an audit of a module or repo.
argument-hint: <test file | package | diff | PR | module | code to plan tests for>
---

# test-challenge

Scope: `$ARGUMENTS`. Resolve it first:
- `PR`, `#123`, `123`, a PR URL → `gh pr diff [n]`; bare `PR` is the current branch's PR.
- Empty → `git diff`, then `git diff --staged`, then `git diff <default-branch>...HEAD`; first non-empty wins. Nothing resolves → say so and stop.
- Files, a package, a directory or code about to get tests → as given.

Khorikov's pillars: **P1** regression protection (restating the code fails it), **P2** refactoring resistance (coupling to implementation detail fails it), **P3** speed, **P4** maintainability.
**P2 is non-negotiable**: failing it means DELETE or CONVERT. P1 and P3 trade off (integration vs unit).

## 1. Mode

- **Review** (file, package, diff, PR): a verdict for every test.
- **Plan** (new code about to get tests; always run it before you generate tests): classify the code, output only the tests worth writing. Zero tests is valid; write "no tests for X" explicitly. Size integration tests by [the integration budget](references/khorikov.md#integration-budget).
- **Audit** (module or repo): a table of test counts per layer × quadrant, the top offenders (most tests, least value), then Review on them.

## 2. Classify the code under test

Read the production code, not only the test. Depth (observable behavior, test doubles, Humble Object): [references/khorikov.md](references/khorikov.md).

| quadrant | complexity | collaborators | tests |
|---|---|---|---|
| Domain model & algorithms | high | few | unit, thorough, through observable behavior |
| Trivial | low | few | none |
| Controllers / orchestration / glue | low | many | a few integration tests, no unit tests |
| Overcomplicated | high | many | none as-is; extract the decision into a pure function or entity method, leave a thin controller |

**Mocks**:
- **Managed** dependency (only this app reads/writes it: own DB, own cache): never mock; real thing in integration tests. Never swap the engine (SQLite/H2 for Postgres, miniredis for a Redis feature it doesn't emulate).
- **Unmanaged** (other systems observe it: bus, SMTP, third-party APIs, webhooks): mock it and assert exact count, exact args, no other calls.
- Mock at the edge: your own adapter closest to the wire, not a wrapper or third-party SDK type. Assert interactions only on mocks, never on stubs.

**Layer rules** (transport / infra / business apps):
1. **Trivial functions** (single if, filter, find, getter, simple mapping): zero tests. Exception: a condition that *is* a business rule belongs on the entity/service and is tested through it, never as a standalone helper.
2. **Transport**: zero unit tests of field mapping. Allowed: a few handler-level tests per endpoint (request in → response out, service faked) for wire names/serialization, validation, status/error mapping, backward compat. One contract/snapshot test beats N per-field asserts.
3. **Infra**: zero unit tests. Managed-dependency repos: integration tests on the real engine (testcontainers, local Postgres/Redis) only for non-trivial queries (complex SQL, transactions, NULLs, migrations, upserts, locking, pagination); trivial CRUD is covered by the controller test. Unmanaged adapters are the mock point; a non-trivial wire mapping inside one gets one contract test against a local stub server.
4. **Business**: home of unit tests. Domain takes values, returns values/decisions; no repository, clock or client injected. Needing a fake of an out-of-process dependency → Overcomplicated (extract and unit-test the decision, integration-test the thin service). Fakes are a fallback, never the default.

## 3. Verdict every test

Every test function, and every table case or group differing in value, gets one verdict, citing its pillar (`[P1] guards the 1000 threshold`, `[P2] asserts the SQL string`):

- **KEEP**: passes P2, earns P1. Check its shape against [references/khorikov.md#shape](references/khorikov.md#shape) first (also before writing any test).
- **DELETE**: fails P2, or adds nothing on P1.
- **CONVERT**: behavior worth guarding, wrong form: unit → integration, managed mock → real engine, interaction assert on stub/managed dep → outcome assert, SDK mock → own-adapter mock, computed expected → hardcoded, per-field mapping → one contract snapshot, helper/private test → public API of the owning entity.
- **MISSING**: an untested business-rule edge, invariant, error mapping, unmanaged interaction or non-trivial query (a happy-path-only rule-heavy unit gets KEEP + MISSING per edge).

**Auto-verdicts:**
- Calls a private/unexported function (incl. Go `export_test.go`, Rust `mod tests` on non-`pub` fns) → DELETE or CONVERT via the public API; too complex to reach → recommend extracting a module with a public API.
- Production code existing only for tests (test flags, `if testing`, test-only setters or `pub(crate)`, exposed private state) → flag **code pollution**.
- Expected value computed with the production algorithm → CONVERT: `assert_eq!(quote(200, 0.15), 170)`, never `200 * (1 - 0.15)`.
- Interaction assert on a stub or managed dependency (`repo.Save` called once), or on the generated SQL string → DELETE [P2], or CONVERT to a stored-state/rows assert.
- Mock of a concrete class or SDK type → CONVERT to your own edge adapter; in-memory substitute for a managed DB → CONVERT to the real engine.
- `time.Now()` / `SystemTime::now()` / random / UUID inside domain code → MISSING refactor: inject a value, not a clock service.
- Assert on a diagnostic log line → DELETE [P2] ([support logging](references/khorikov.md#logging) is an unmanaged dependency).
- Coverage % as the reason for a test → ignore.
- **Guardrail:** the only coverage of a real business rule is CONVERT (onto the owning entity/service), never DELETE.

Be opinionated. "It depends" only with the concrete reason: "KEEP if `/v1/users` is consumed by mobile clients pinned to old builds".

## 4. Report

```
## DELETE (9)
- `user_handler_test.go:TestToUserDTO_Email` — transport · trivial — [P1] restates `toUserDTO`; catches nothing.

## CONVERT (2)
- `order_repo_test.rs:finds_overdue_orders` — infra · managed DB — [P2] mockall asserts the SQL string; run it against Postgres instead.
  <short sketch of the replacement test>

## KEEP (2)
- `checkout.rs:placing_an_order_publishes_order_placed` — business · controller — [P1] the bus is unmanaged; asserts exactly one `OrderPlaced` with these args and nothing else.

## MISSING (3)
- `pricing_test.go:TestQuote_DiscountThreshold` — business · domain — [P1] 1000 is the rule edge: 999.99 / 1000 / 1000.01.
  <short sketch>

Net: −9 deleted, 2 converted, +3 added (13 → 7 tests).
```

- Sections in this order; line format `file:test name` — layer · quadrant — `[Pn]` reason; code-pollution flags on the causing test's line. Plan mode: only MISSING, plus which units get none and why.
- Sketches use hardcoded expected values and the stack's idiom: [references/go.md](references/go.md), [references/rust.md](references/rust.md); other languages map the same shapes.
- Review and Audit report only; edit or delete test files once the user says to apply. Plan mode, when you were about to write tests: write exactly the planned ones.
