# Go shapes

Uses `testing`, `net/http/httptest`, testify `require` (plain `t.Fatalf` if the repo lacks it) and `testcontainers-go`.

## CONVERT per-field mapping tests → handler contract test

`TestToUserDTO_Email`-style tests restate `toUserDTO` [P1] and call an unexported function [P2].
Replace them with one table test per endpoint, service faked in-process; the expected body is a literal (or golden file under `testdata/`), pinning the wire format, not the Go struct.

```go
type fakeUsers struct{ err error }

func (f fakeUsers) Create(_ context.Context, in domain.NewUser) (domain.User, error) {
	if f.err != nil {
		return domain.User{}, f.err
	}
	return domain.User{ID: 42, Email: in.Email, Name: in.Name}, nil
}

func TestCreatingAUser(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		svcErr   error
		wantCode int
		wantBody string
	}{
		{"valid request returns the created user", `{"email":"a@b.c","name":"Ann"}`, nil,
			http.StatusCreated, `{"id":42,"email":"a@b.c","name":"Ann"}`},
		{"malformed email is a 400", `{"email":"nope","name":"Ann"}`, nil,
			http.StatusBadRequest, `{"error":"invalid_email"}`},
		{"taken email is a 409", `{"email":"a@b.c","name":"Ann"}`, domain.ErrEmailTaken,
			http.StatusConflict, `{"error":"email_taken"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(fakeUsers{err: tt.svcErr})
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/users", strings.NewReader(tt.body)))

			require.Equal(t, tt.wantCode, rec.Code)
			require.JSONEq(t, tt.wantBody, rec.Body.String())
		})
	}
}
```

## KEEP / MISSING: domain table test

```go
func TestOrderDiscount(t *testing.T) {
	tests := []struct {
		name    string
		total   money.Amount
		blocked bool
		want    money.Amount
	}{
		{"999.99 gets no discount", money.New(999_99), false, money.New(0)},
		{"exactly 1000 gets no discount", money.New(1000_00), false, money.New(0)},
		{"1000.01 gets 50", money.New(1000_01), false, money.New(50_00)},
		{"blocked customer gets no discount", money.New(5000_00), true, money.New(0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := domain.NewOrder(domain.Customer{Blocked: tt.blocked}, tt.total)

			require.Equal(t, tt.want, o.Discount())
		})
	}
}
```

`want := total.Mul(0.05)` repeats the formula [P1]: hardcode `money.New(50_00)`.
A rule in `utils.IsDiscountEligible(o)` with its own test → CONVERT onto `Order` and into this table.

## Functional core instead of a fake repository

Extract the decision of an Overcomplicated service into a pure function taking values, including the current time:

```go
func Cancel(o Order, now time.Time) (Order, Refund, error) {
	if o.Status != StatusPaid {
		return o, Refund{}, ErrNotPaid
	}
	if now.Sub(o.PaidAt) > 14*24*time.Hour {
		return o, Refund{}, ErrRefundWindowClosed
	}
	o.Status = StatusRefunded
	return o, Refund{OrderID: o.ID, Cents: o.TotalCents}, nil
}
```

```go
func TestCancel(t *testing.T) {
	paidAt := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	paid := Order{ID: 1, Status: StatusPaid, PaidAt: paidAt, TotalCents: 120_00}
	tests := []struct {
		name       string
		now        time.Time
		wantStatus Status
		wantRefund Refund
		wantErr    error
	}{
		{"cancel on day 14 refunds in full", time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
			StatusRefunded, Refund{OrderID: 1, Cents: 120_00}, nil},
		{"cancel one second after day 14 is rejected", time.Date(2026, 3, 15, 10, 0, 1, 0, time.UTC),
			StatusPaid, Refund{}, ErrRefundWindowClosed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, refund, err := Cancel(paid, tt.now)

			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, tt.wantStatus, got.Status)
			require.Equal(t, tt.wantRefund, refund)
		})
	}
}
```

The service becomes a thin shell (load, `Cancel(o, s.now())`, save, call the payments adapter), covered by one integration test.
`time.Since(o.PaidAt)` inside the entity is a MISSING refactor: pass `now time.Time`; an injected `Clock` interface is the same problem.

## DELETE: interaction assertion on a managed dependency

```go
repo.EXPECT().Save(gomock.Any(), gomock.Any()).Times(1)
```

Own DB is managed: the call is implementation detail [P2] and passes with the wrong status. Assert the stored row in the integration test.

## KEEP: mock of an unmanaged dependency at the edge

Mock your own adapter (`EventBus`), never the SDK (`*kafka.Writer`, `*sns.Client`).

```go
type EventBus interface {
	Publish(ctx context.Context, e OrderPlaced) error
}

type spyBus struct{ published []OrderPlaced }

func (s *spyBus) Publish(_ context.Context, e OrderPlaced) error {
	s.published = append(s.published, e)
	return nil
}

func TestPlacingAnOrderPublishesOrderPlaced(t *testing.T) {
	db := newDB(t)
	seedCustomer(t, db, 7)
	bus := &spyBus{}
	checkout := service.NewCheckout(postgres.NewOrders(db), bus, fixedNow)

	id, err := checkout.Place(ctx, 7, []domain.Line{{SKU: "sku-1", Qty: 2, UnitCents: 15_00}})

	require.NoError(t, err)
	require.Equal(t, []OrderPlaced{{OrderID: id, CustomerID: 7, TotalCents: 30_00}}, bus.published)
	require.EqualValues(t, 30_00, orderTotal(t, db, id))
}
```

The whole-slice compare asserts exact count, args and no other calls; gomock equivalent: `bus.EXPECT().Publish(gomock.Any(), OrderPlaced{OrderID: 1, CustomerID: 7, TotalCents: 30_00}).Times(1)` on a controller failing on unexpected calls.
This doubles as the scenario's one happy-path integration test.

## Code pollution: `export_test.go`

```go
package pricing

var ApplyTiers = applyTiers
```

Calling `pricing.ApplyTiers` tests a private function [P2]: flag code pollution, CONVERT to a test through `pricing.Quote`, or extract `pricing/tiers` with an exported `Apply` if too complex. Same flag on `if testing.Testing()`, test-only exported setters and fields.

## Integration test on real Postgres

One container per package, real migrations, cleanup at test start; build tag keeps `go test ./...` fast; tests sharing the pool don't call `t.Parallel()`.

```go
//go:build integration

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("app"),
		postgres.WithUsername("app"),
		postgres.WithPassword("app"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		log.Fatal(err)
	}
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	if err := migrations.Up(dsn); err != nil {
		log.Fatal(err)
	}
	if pool, err = pgxpool.New(ctx, dsn); err != nil {
		log.Fatal(err)
	}
	code := m.Run()
	pool.Close()
	_ = ctr.Terminate(ctx)
	os.Exit(code)
}

func newDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	_, err := pool.Exec(context.Background(), `TRUNCATE customers, orders, payments, stock RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
	return pool
}
```

Arrange, act and assert each in their own transaction:

```go
func TestOverdueSearchSkipsPaidOrders(t *testing.T) {
	db := newDB(t)
	late := insertOrder(t, db, order{DueAt: day(1), Status: "open"})
	insertOrder(t, db, order{DueAt: day(1), Status: "paid"})
	insertOrder(t, db, order{DueAt: day(9), Status: "open"})

	got, err := NewOrderRepo(db).Overdue(ctx, day(5))

	require.NoError(t, err)
	require.Equal(t, []int64{late}, ids(got))
}

func TestReservingStockNeverOversells(t *testing.T) {
	db := newDB(t)
	insertStock(t, db, "sku-1", 5)

	succeeded := reserveConcurrently(NewStockRepo(db), "sku-1", 10)

	require.Equal(t, 5, succeeded)
	require.Zero(t, availableStock(t, db, "sku-1"))
}
```

Helpers each run one committed statement; `reserveConcurrently` keeps goroutines out of the test body.
SQLite or `go-sqlmock` for a Postgres schema → CONVERT to this shape [P1] (no locking, NULL or `ON CONFLICT` fidelity).
