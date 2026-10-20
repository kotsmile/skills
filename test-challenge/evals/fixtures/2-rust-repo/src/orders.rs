use chrono::{DateTime, Utc};

#[cfg_attr(test, mockall::automock)]
#[async_trait::async_trait]
pub trait Db: Send + Sync {
    async fn fetch_orders(&self, sql: &str, args: &[Arg]) -> Result<Vec<Order>, DbError>;
    async fn execute(&self, sql: &str, args: &[Arg]) -> Result<u64, DbError>;
}

pub struct OrderRepo<D: Db> {
    db: D,
}

pub struct Filter {
    pub customer_id: Option<i64>,
    pub statuses: Vec<Status>,
    pub due_before: Option<DateTime<Utc>>,
    pub min_total_cents: Option<i64>,
}

impl<D: Db> OrderRepo<D> {
    pub fn new(db: D) -> Self {
        Self { db }
    }

    pub async fn get(&self, id: i64) -> Result<Option<Order>, DbError> {
        Ok(self.db.fetch_orders("SELECT * FROM orders WHERE id = $1", &[Arg::I64(id)]).await?.pop())
    }

    pub async fn search(&self, f: &Filter) -> Result<Vec<Order>, DbError> {
        let mut sql = String::from("SELECT o.* FROM orders o LEFT JOIN payments p ON p.order_id = o.id WHERE 1=1");
        let mut args = Vec::new();
        if let Some(c) = f.customer_id {
            args.push(Arg::I64(c));
            sql += &format!(" AND o.customer_id = ${}", args.len());
        }
        if !f.statuses.is_empty() {
            args.push(Arg::Statuses(f.statuses.clone()));
            sql += &format!(" AND o.status = ANY(${})", args.len());
        }
        if let Some(d) = f.due_before {
            args.push(Arg::Ts(d));
            sql += &format!(" AND o.due_at < ${} AND p.id IS NULL", args.len());
        }
        if let Some(m) = f.min_total_cents {
            args.push(Arg::I64(m));
            sql += &format!(" AND o.total_cents >= ${}", args.len());
        }
        sql += " ORDER BY o.due_at NULLS LAST, o.id";
        self.db.fetch_orders(&sql, &args).await
    }

    pub async fn mark_paid_once(&self, id: i64, payment_ref: &str) -> Result<bool, DbError> {
        let n = self
            .db
            .execute(
                "INSERT INTO payments (order_id, ref) VALUES ($1, $2) ON CONFLICT (order_id) DO NOTHING",
                &[Arg::I64(id), Arg::Str(payment_ref.into())],
            )
            .await?;
        Ok(n == 1)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn get_uses_id_query() {
        let mut db = MockDb::new();
        db.expect_fetch_orders()
            .withf(|sql, _| sql == "SELECT * FROM orders WHERE id = $1")
            .times(1)
            .returning(|_, _| Ok(vec![]));
        assert!(OrderRepo::new(db).get(1).await.unwrap().is_none());
    }

    #[tokio::test]
    async fn search_by_customer_adds_clause() {
        let mut db = MockDb::new();
        db.expect_fetch_orders()
            .withf(|sql, args| sql.contains("AND o.customer_id = $1") && args.len() == 1)
            .times(1)
            .returning(|_, _| Ok(vec![]));
        OrderRepo::new(db).search(&Filter { customer_id: Some(5), statuses: vec![], due_before: None, min_total_cents: None }).await.unwrap();
    }

    #[tokio::test]
    async fn search_overdue_adds_unpaid_clause() {
        let mut db = MockDb::new();
        db.expect_fetch_orders()
            .withf(|sql, _| sql.contains("o.due_at < $1 AND p.id IS NULL"))
            .times(1)
            .returning(|_, _| Ok(vec![]));
        OrderRepo::new(db).search(&Filter { customer_id: None, statuses: vec![], due_before: Some(Utc::now()), min_total_cents: None }).await.unwrap();
    }

    #[tokio::test]
    async fn search_all_filters_numbers_params_in_order() {
        let mut db = MockDb::new();
        db.expect_fetch_orders()
            .withf(|sql, args| sql.contains("$4") && args.len() == 4)
            .times(1)
            .returning(|_, _| Ok(vec![]));
        OrderRepo::new(db)
            .search(&Filter { customer_id: Some(1), statuses: vec![Status::Open], due_before: Some(Utc::now()), min_total_cents: Some(100) })
            .await
            .unwrap();
    }

    #[tokio::test]
    async fn mark_paid_once_returns_true_on_insert() {
        let mut db = MockDb::new();
        db.expect_execute().times(1).returning(|_, _| Ok(1));
        assert!(OrderRepo::new(db).mark_paid_once(1, "r").await.unwrap());
    }
}
