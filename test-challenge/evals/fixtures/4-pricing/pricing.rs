pub struct Customer {
    pub blocked: bool,
    pub loyalty_years: u32,
}

pub struct Quote {
    pub subtotal_cents: i64,
    pub discount_cents: i64,
    pub total_cents: i64,
}

pub struct PricingService<P: PromoStore> {
    promos: P,
}

impl<P: PromoStore> PricingService<P> {
    pub async fn quote(&self, c: &Customer, items: &[Line], promo: Option<&str>) -> Result<Quote, PricingError> {
        let subtotal: i64 = items.iter().map(|l| l.unit_cents * l.qty as i64).sum();
        let mut pct = 0;
        if subtotal > 100_000 && !c.blocked {
            pct += 5;
        }
        if c.loyalty_years >= 3 {
            pct += 2;
        }
        if let Some(code) = promo {
            let p = self.promos.find(code).await?.ok_or(PricingError::UnknownPromo)?;
            if p.expired() {
                return Err(PricingError::ExpiredPromo);
            }
            pct += p.percent;
        }
        let pct = pct.min(15);
        let discount = subtotal * pct as i64 / 100;
        Ok(Quote { subtotal_cents: subtotal, discount_cents: discount, total_cents: subtotal - discount })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn quote_happy_path() {
        let svc = PricingService { promos: InMemoryPromos::default() };
        let c = Customer { blocked: false, loyalty_years: 0 };
        let q = svc.quote(&c, &[Line { unit_cents: 2_000, qty: 3 }], None).await.unwrap();
        assert_eq!(q.total_cents, 6_000);
    }
}
