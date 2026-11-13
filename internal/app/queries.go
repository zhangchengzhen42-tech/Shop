package app

import (
	"context"
	"database/sql"
)

func (s *Store) User(ctx context.Context, id string) (User, error) {
	var user User
	err := s.db.QueryRowContext(ctx, "SELECT id,name,role FROM users WHERE id=?", id).Scan(&user.ID, &user.Name, &user.Role)
	if err == sql.ErrNoRows {
		return User{}, ErrNotFound
	}
	return user, err
}

func (s *Store) Products(ctx context.Context) ([]Product, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,name,price_cents,available,on_sale FROM products ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []Product
	for rows.Next() {
		var item Product
		if err := rows.Scan(&item.ID, &item.Name, &item.Price, &item.Available, &item.OnSale); err != nil {
			return nil, err
		}
		products = append(products, item)
	}
	return products, rows.Err()
}

func (s *Store) Orders(ctx context.Context, user User) ([]Order, error) {
	query := `SELECT o.id,o.buyer_id,u.name,o.product_id,o.product_name,o.unit_price,o.quantity,o.total_cents,o.status,o.created_at,o.close_at,o.closed_at,COALESCE(o.close_reason,''),COALESCE(o.payment_tx,''),o.paid_at,COALESCE(s.carrier,''),COALESCE(s.tracking,'') FROM orders o JOIN users u ON u.id=o.buyer_id LEFT JOIN shipments s ON s.order_id=o.id`
	args := []any{}
	if user.Role == RoleBuyer {
		query += " WHERE o.buyer_id=?"
		args = append(args, user.ID)
	}
	query += " ORDER BY o.created_at DESC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var orders []Order
	for rows.Next() {
		item, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		orders = append(orders, item)
	}
	return orders, rows.Err()
}

func scanOrder(row interface{ Scan(...any) error }) (Order, error) {
	var item Order
	var created, closeAt string
	var closed, paid sql.NullString
	err := row.Scan(&item.ID, &item.BuyerID, &item.BuyerName, &item.ProductID, &item.ProductName, &item.UnitPrice, &item.Quantity, &item.Total, &item.Status, &created, &closeAt, &closed, &item.CloseReason, &item.PaymentTx, &paid, &item.Carrier, &item.Tracking)
	if err != nil {
		return Order{}, err
	}
	if item.CreatedAt, err = parseTime(created); err != nil {
		return Order{}, err
	}
	if item.CloseAt, err = parseTime(closeAt); err != nil {
		return Order{}, err
	}
	if closed.Valid {
		value, err := parseTime(closed.String)
		if err != nil {
			return Order{}, err
		}
		item.ClosedAt = &value
	}
	if paid.Valid {
		value, err := parseTime(paid.String)
		if err != nil {
			return Order{}, err
		}
		item.PaidAt = &value
	}
	return item, nil
}

func (s *Store) Payments(ctx context.Context) ([]Payment, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT COALESCE(order_id,''),channel_tx,amount_cents,verification_status,result,COALESCE(reason,''),received_at FROM payments ORDER BY received_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Payment
	for rows.Next() {
		var item Payment
		var received string
		if err := rows.Scan(&item.OrderID, &item.ChannelTx, &item.Amount, &item.VerificationStatus, &item.Result, &item.Reason, &received); err != nil {
			return nil, err
		}
		if item.ReceivedAt, err = parseTime(received); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Refunds(ctx context.Context) ([]Refund, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,COALESCE(order_id,''),COALESCE(exception_id,''),kind,amount_cents,status,COALESCE(reason,''),created_at,updated_at FROM refunds ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var refunds []Refund
	for rows.Next() {
		var item Refund
		var created, updated string
		if err := rows.Scan(&item.ID, &item.OrderID, &item.ExceptionID, &item.Kind, &item.Amount, &item.Status, &item.Reason, &created, &updated); err != nil {
			return nil, err
		}
		var err error
		if item.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		if item.UpdatedAt, err = parseTime(updated); err != nil {
			return nil, err
		}
		refunds = append(refunds, item)
	}
	return refunds, rows.Err()
}

func (s *Store) Exceptions(ctx context.Context) ([]Exception, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,order_id,channel_tx,amount_cents,kind,status,reason,created_at FROM exceptions ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Exception
	for rows.Next() {
		var item Exception
		var created string
		if err := rows.Scan(&item.ID, &item.OrderID, &item.ChannelTx, &item.Amount, &item.Kind, &item.Status, &item.Reason, &created); err != nil {
			return nil, err
		}
		var err error
		if item.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Shipments(ctx context.Context) ([]Shipment, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT order_id,carrier,tracking,created_at FROM shipments ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Shipment
	for rows.Next() {
		var item Shipment
		var created string
		if err := rows.Scan(&item.OrderID, &item.Carrier, &item.Tracking, &created); err != nil {
			return nil, err
		}
		var err error
		if item.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Audits(ctx context.Context) ([]Audit, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT created_at,actor,action,detail FROM audits ORDER BY id DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Audit
	for rows.Next() {
		var item Audit
		var created string
		if err := rows.Scan(&created, &item.Actor, &item.Action, &item.Detail); err != nil {
			return nil, err
		}
		var err error
		if item.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) Summary(ctx context.Context) (Summary, error) {
	var result Summary
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(available),0) FROM products").Scan(&result.Available); err != nil {
		return result, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(amount_cents),0) FROM payments WHERE result IN ('ACCEPTED','LATE')").Scan(&result.ReceivedCents); err != nil {
		return result, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(amount_cents),0) FROM refunds WHERE status=?", RefundSuccess).Scan(&result.RefundedCents); err != nil {
		return result, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM orders WHERE status=?", OrderAwaitShipment).Scan(&result.AwaitShipment); err != nil {
		return result, err
	}
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM orders WHERE status=?", OrderPending).Scan(&result.PendingPayment); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) Dashboard(ctx context.Context, user User) (Dashboard, error) {
	var result Dashboard
	var err error
	if result.Products, err = s.Products(ctx); err != nil {
		return result, err
	}
	if result.Orders, err = s.Orders(ctx, user); err != nil {
		return result, err
	}
	if user.Role == RoleBuyer {
		return result, nil
	}
	if result.Payments, err = s.Payments(ctx); err != nil {
		return result, err
	}
	if result.Refunds, err = s.Refunds(ctx); err != nil {
		return result, err
	}
	if result.Shipments, err = s.Shipments(ctx); err != nil {
		return result, err
	}
	if result.Exceptions, err = s.Exceptions(ctx); err != nil {
		return result, err
	}
	result.Audits, err = s.Audits(ctx)
	return result, err
}
