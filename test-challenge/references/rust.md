# Rust shapes

Unit tests in `#[cfg(test)] mod tests` beside the code; integration tests in `tests/`, public API only.
Uses `tokio`, `axum`, `sqlx`, `insta`, `mockall`, `testcontainers-modules`.

## DELETE: mockall on your own repository or query builder

```rust
#[tokio::test]
async fn find_overdue_builds_query() {
    let mut db = MockDb::new();
    db.expect_query()
        .withf(|sql, _| sql.contains("WHERE due_at < $1 AND status <> 'paid'"))
        .times(1)
        .returning(|_, _| Ok(vec![]));
    OrderRepo::new(db).find_overdue(now()).await.unwrap();
}
```

Asserts SQL text [P2] and passes wrong `<>` NULL semantics [P1]: CONVERT to Postgres. Same for `sqlx::SqlitePool` on a Postgres schema.

## CONVERT → integration test in `tests/`

`#[sqlx::test]` gives a fresh database per test with `migrations/` applied; arrange / act / assert in separate transactions. In `tests/order_repo.rs`:

```rust
use app::orders::{OrderRepo, Status};
use sqlx::PgPool;

async fn insert_order(pool: &PgPool, due: &str, status: &str) -> i64 {
    let mut tx = pool.begin().await.unwrap();
    let id = sqlx::query_scalar("INSERT INTO orders (due_at, status) VALUES ($1::timestamptz, $2) RETURNING id")
        .bind(due)
        .bind(status)
        .fetch_one(&mut *tx)
        .await
        .unwrap();
    tx.commit().await.unwrap();
    id
}

#[sqlx::test]
async fn overdue_search_skips_paid_orders_and_missing_due_dates(pool: PgPool) {
    let late = insert_order(&pool, "2026-03-01T00:00:00Z", "open").await;
    insert_order(&pool, "2026-03-01T00:00:00Z", "paid").await;
    insert_order(&pool, "2026-03-20T00:00:00Z", "open").await;

    let found = OrderRepo::new(pool.clone()).find_overdue(at("2026-03-10T00:00:00Z")).await.unwrap();

    assert_eq!(found.iter().map(|o| o.id).collect::<Vec<_>>(), vec![late]);
}
```

Without `DATABASE_URL`, a container per test:

```rust
use testcontainers_modules::{postgres::Postgres, testcontainers::{runners::AsyncRunner, ContainerAsync}};

async fn fresh_db() -> (PgPool, ContainerAsync<Postgres>) {
    let node = Postgres::default().start().await.unwrap();
    let port = node.get_host_port_ipv4(5432).await.unwrap();
    let pool = PgPool::connect(&format!("postgres://postgres:postgres@127.0.0.1:{port}/postgres")).await.unwrap();
    sqlx::migrate!().run(&pool).await.unwrap();
    (pool, node)
}
```

Keep the container handle alive; dropping it stops Postgres.

## CONVERT → handler contract snapshot

Per-field `assert_eq!(dto.email, user.email)` tests become one snapshot per endpoint behavior, service faked in-process behind its trait; the `snapshots/` file is the reviewed wire contract.

```rust
#[cfg(test)]
mod tests {
    use super::*;
    use axum::{body::Body, http::{Request, StatusCode}};
    use tower::ServiceExt;

    struct FakeUsers(Result<User, UserError>);

    impl UserService for FakeUsers {
        async fn create(&self, _: NewUser) -> Result<User, UserError> {
            self.0.clone()
        }
    }

    async fn post_user(svc: FakeUsers, body: &str) -> (StatusCode, serde_json::Value) {
        let res = router(Arc::new(svc))
            .oneshot(
                Request::post("/v1/users")
                    .header("content-type", "application/json")
                    .body(Body::from(body.to_owned()))
                    .unwrap(),
            )
            .await
            .unwrap();
        let status = res.status();
        let bytes = axum::body::to_bytes(res.into_body(), usize::MAX).await.unwrap();
        (status, serde_json::from_slice(&bytes).unwrap())
    }

    #[tokio::test]
    async fn created_user_matches_the_wire_contract() {
        let (status, body) = post_user(FakeUsers(Ok(user(42))), r#"{"email":"a@b.c","name":"Ann"}"#).await;

        assert_eq!(status, StatusCode::CREATED);
        insta::assert_json_snapshot!(body);
    }

    #[tokio::test]
    async fn taken_email_is_a_409() {
        let (status, body) = post_user(FakeUsers(Err(UserError::EmailTaken)), r#"{"email":"a@b.c","name":"Ann"}"#).await;

        assert_eq!(status, StatusCode::CONFLICT);
        assert_eq!(body, serde_json::json!({"error": "email_taken"}));
    }
}
```

## KEEP / MISSING: domain unit tests

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn discount_applies_only_above_1000_for_unblocked_customers() {
        let cases = [
            (cents(999_99), false, cents(0)),
            (cents(1000_00), false, cents(0)),
            (cents(1000_01), false, cents(50_00)),
            (cents(5000_00), true, cents(0)),
        ];
        for (total, blocked, want) in cases {
            let order = Order::new(Customer { blocked }, total);

            assert_eq!(order.discount(), want, "total={total:?} blocked={blocked}");
        }
    }
}
```

Use `rstest` `#[case(...)]` for the same shape only if the crate already depends on it.

## Functional core instead of a fake store

Extract the decision of an Overcomplicated service (needs an in-memory `OrderStore`), taking the current time as a value:

```rust
pub fn cancel(order: &Order, now: DateTime<Utc>) -> Result<(Order, Refund), CancelError> {
    if order.status != Status::Paid {
        return Err(CancelError::NotPaid);
    }
    if now - order.paid_at > Duration::days(14) {
        return Err(CancelError::RefundWindowClosed);
    }
    Ok((
        Order { status: Status::Refunded, ..order.clone() },
        Refund { order_id: order.id, cents: order.total_cents },
    ))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn paid() -> Order {
        Order { id: 1, status: Status::Paid, paid_at: at("2026-03-01T10:00:00Z"), total_cents: 120_00 }
    }

    #[test]
    fn cancelling_on_day_14_refunds_in_full() {
        let (order, refund) = cancel(&paid(), at("2026-03-15T10:00:00Z")).unwrap();

        assert_eq!(order.status, Status::Refunded);
        assert_eq!(refund, Refund { order_id: 1, cents: 120_00 });
    }

    #[test]
    fn cancelling_after_day_14_is_rejected() {
        let result = cancel(&paid(), at("2026-03-15T10:00:01Z"));

        assert_eq!(result, Err(CancelError::RefundWindowClosed));
    }
}
```

`Utc::now()` (or `SystemTime::now()`, `rand`, `Uuid::new_v4()`) inside the entity is a MISSING refactor: pass `now`; an injected `Clock` trait is the same problem.
The service becomes a thin shell (load, `cancel(&order, Utc::now())`, save, call the payments adapter), covered by one integration test.

## KEEP: mock of an unmanaged dependency at the edge

Mock your own trait (`EventBus`), never the SDK (`rdkafka::producer::FutureProducer`, `aws_sdk_sns::Client`). In-crate, `mockall` panics on unexpected calls, so `times(1)` + `with` asserts count, args and nothing else:

```rust
#[cfg_attr(test, mockall::automock)]
pub trait EventBus: Send + Sync {
    fn publish(&self, event: OrderPlaced) -> Result<(), BusError>;
}

#[test]
fn placing_an_order_publishes_order_placed() {
    let mut bus = MockEventBus::new();
    bus.expect_publish()
        .with(mockall::predicate::eq(OrderPlaced { order_id: 1, customer_id: 7, total_cents: 30_00 }))
        .times(1)
        .returning(|_| Ok(()));

    place_order(&bus, draft(1, 7, 30_00)).unwrap();
}
```

`#[cfg(test)]` items don't exist in `tests/`, so the integration test uses a recording spy:

```rust
#[derive(Default)]
struct SpyBus(Mutex<Vec<OrderPlaced>>);

impl EventBus for SpyBus {
    fn publish(&self, event: OrderPlaced) -> Result<(), BusError> {
        self.0.lock().unwrap().push(event);
        Ok(())
    }
}

#[sqlx::test]
async fn checkout_stores_the_order_and_publishes_order_placed(pool: PgPool) {
    insert_customer(&pool, 7).await;
    let bus = SpyBus::default();

    let id = Checkout::new(pool.clone(), &bus).place(7, &[line("sku-1", 2, 15_00)]).await.unwrap();

    assert_eq!(*bus.0.lock().unwrap(), vec![OrderPlaced { order_id: id, customer_id: 7, total_cents: 30_00 }]);
    assert_eq!(order_total(&pool, id).await, 30_00);
}
```

## Code pollution: reaching private items

```rust
fn apply_tiers(lines: &[Line]) -> Cents { ... }

#[cfg(test)]
mod tests {
    #[test]
    fn apply_tiers_sums_bands() { assert_eq!(super::apply_tiers(&lines()), cents(42_00)); }
}
```

Private item [P2]: CONVERT through the public `quote`, or extract `pricing::tiers` with `pub fn apply` if too complex. Code pollution: test-only `pub(crate)`, `#[cfg(test)] pub fn set_status(...)` setters, fields made `pub` for tests.
