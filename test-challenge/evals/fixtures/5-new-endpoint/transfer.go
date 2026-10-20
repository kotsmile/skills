package transfer

type Handler struct{ svc *Service }

type transferReq struct {
	FromAccount string `json:"from_account"`
	ToAccount   string `json:"to_account"`
	AmountCents int64  `json:"amount_cents"`
	IdemKey     string `json:"idempotency_key"`
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req transferReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_json")
		return
	}
	t, err := h.svc.Transfer(r.Context(), req.IdemKey, req.FromAccount, req.ToAccount, req.AmountCents)
	switch {
	case errors.Is(err, ErrInsufficientFunds):
		writeErr(w, http.StatusUnprocessableEntity, "insufficient_funds")
	case errors.Is(err, ErrSameAccount), errors.Is(err, ErrNonPositiveAmount):
		writeErr(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrAccountFrozen):
		writeErr(w, http.StatusForbidden, "account_frozen")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, "internal")
	default:
		writeJSON(w, http.StatusCreated, map[string]any{"id": t.ID, "status": t.Status})
	}
}

type Repo interface {
	GetAccount(ctx context.Context, id string) (Account, error)
	FindByIdemKey(ctx context.Context, key string) (Transfer, bool, error)
	ApplyTransfer(ctx context.Context, t Transfer) error
}

type Service struct {
	repo  Repo
	limit int64
}

func (s *Service) Transfer(ctx context.Context, key, from, to string, amount int64) (Transfer, error) {
	if amount <= 0 {
		return Transfer{}, ErrNonPositiveAmount
	}
	if from == to {
		return Transfer{}, ErrSameAccount
	}
	if t, ok, err := s.repo.FindByIdemKey(ctx, key); err != nil || ok {
		return t, err
	}
	src, err := s.repo.GetAccount(ctx, from)
	if err != nil {
		return Transfer{}, err
	}
	if src.Frozen {
		return Transfer{}, ErrAccountFrozen
	}
	if src.BalanceCents+src.OverdraftCents < amount {
		return Transfer{}, ErrInsufficientFunds
	}
	status := StatusDone
	if amount > s.limit {
		status = StatusPendingReview
	}
	t := Transfer{ID: newID(), IdemKey: key, From: from, To: to, AmountCents: amount, Status: status}
	return t, s.repo.ApplyTransfer(ctx, t)
}

type PgRepo struct{ db *pgxpool.Pool }

func (r *PgRepo) GetAccount(ctx context.Context, id string) (Account, error) {
	var a Account
	err := r.db.QueryRow(ctx, `SELECT id, balance_cents, overdraft_cents, frozen FROM accounts WHERE id = $1`, id).
		Scan(&a.ID, &a.BalanceCents, &a.OverdraftCents, &a.Frozen)
	return a, err
}

func (r *PgRepo) FindByIdemKey(ctx context.Context, key string) (Transfer, bool, error) {
	var t Transfer
	err := r.db.QueryRow(ctx, `SELECT id, status FROM transfers WHERE idem_key = $1`, key).Scan(&t.ID, &t.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Transfer{}, false, nil
	}
	return t, err == nil, err
}

func (r *PgRepo) ApplyTransfer(ctx context.Context, t Transfer) error {
	return pgx.BeginTxFunc(ctx, r.db, pgx.TxOptions{IsoLevel: pgx.Serializable}, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE accounts SET balance_cents = balance_cents - $2
			WHERE id = $1 AND balance_cents + overdraft_cents >= $2`, t.From, t.AmountCents)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrInsufficientFunds
		}
		if _, err := tx.Exec(ctx, `UPDATE accounts SET balance_cents = balance_cents + $2 WHERE id = $1`, t.To, t.AmountCents); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO transfers (id, idem_key, from_acc, to_acc, amount_cents, status)
			VALUES ($1, $2, $3, $4, $5, $6)`, t.ID, t.IdemKey, t.From, t.To, t.AmountCents, t.Status)
		return err
	})
}
