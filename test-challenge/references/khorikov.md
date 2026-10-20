# Khorikov's model

Source: Vladimir Khorikov, *Unit Testing: Principles, Practices, and Patterns* (Manning, 2020). Pillars and quadrant table: SKILL.md.

## Quadrants in depth

- **Complexity / domain significance**: branching, calculations, invariants, business rules. A single `if` is low unless it is a rule the business cares about.
- **Collaborators**: mutable or out-of-process dependencies (DB, queue, HTTP client, clock, other services), not immutable values passed in.
- **Domain** = **functional core**: takes values (order, customer, current time, promo already loaded), returns values or decisions (quote, refund, error, events to publish). Every rule edge is a one-line table case.
- **Controllers** = **imperative shell**: load values, hand them to the domain, act on the decision (persist, publish, send). Covered by the [integration budget](#integration-budget).
- **Overcomplicated**: logic tangled with I/O; the tell is that a unit test needs a fake or mock of an out-of-process dependency. Recommend the **Humble Object** split: decisions into a pure function or entity method (unit-tested), a thin controller that fetches, calls the domain, persists/publishes (integration-tested). An in-memory fake repo is a fallback, never the default.

## Observable behavior vs implementation detail

Observable: return values, resulting state, side effects visible *outside the application* (a row other systems read, a message published, an email sent).
Implementation detail: private methods, which internal collaborator got called how often, intermediate structures, the SQL text, writes to your own DB seen through its repository.

## Test doubles

- **Stub**: canned incoming data. Never assert on a stub. **Mock**: verifies an outgoing call. **Fake**: working lightweight implementation.
- Managed dependency: only the final state matters; never mock or fake it in integration tests.
- Mock the adapter you own closest to the wire (`EventBus.Publish(OrderPlaced)`, `Mailer.Send(Email)`), never the SDK (`*stripe.Client`, `aws_sdk_sqs::Client`): mocking the SDK pins its call shape and leaves your mapping untested.
- A DB shared with another app is unmanaged for the tables that app reads; the rest stays managed.
- Never mock between domain classes (classical school: isolate tests, not units).

## Shape

Every kept or newly written test:

- **Exactly one Act line.** 2+ calls to act means the API leaks invariants: flag the production API, not the test.
- **One Arrange/Act/Assert cycle.** Repeated Act/Assert → split, or it is an integration test.
- **No `if` / `match` / loops in the body**, except iterating the case table.
- **Multiple asserts are fine** for one unit of behavior.
- **Names are plain-English facts**: `delivery_with_past_date_is_invalid`, not `TestValidate_PastDate_ReturnsFalse`; no method names.
- **Hardcoded expected values** from the spec or worked by hand.
- **Shared setup only via factory functions or builders**, never shared mutable fixtures or suite-level state.
- **DB integration tests**: arrange, act and assert each in their own transaction; clean up at test *start*, not end; schema from the real migrations, never a hand-written test schema.

## Integration budget

- Per business scenario: **one happy-path integration test** touching every out-of-process dependency it uses, plus edge cases no unit test can reach.
- Every edge case a unit test can cover goes in a unit test.
- Edge cases reachable only through integration: check for **fail-fast** first; an early error/panic precondition often needs no test.
- Repositories are not tested in isolation: the controller test covers writes; read queries get their own integration test only when non-trivial (joins, aggregations, pagination, locking, NULL semantics).

## Logging

- **Support logging** (audit trail, ops events someone acts on) is observable: test it as an unmanaged dependency through a domain-level abstraction (`AuditLog.OrderRefunded(id, amount)`) with exact-call assertions.
- **Diagnostic logging** is never tested; a test asserting it → DELETE [P2].
