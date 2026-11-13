package app

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) CreateOrder(ctx context.Context, buyer User, productID string, quantity int) (Order, error) {
	if buyer.Role != RoleBuyer {
		return Order{}, ErrForbidden
	}
	if quantity < 1 || quantity > 2 {
		return Order{}, ErrInvalidQuantity
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback()

	var product Product
	err = tx.QueryRowContext(ctx, "SELECT id,name,price_cents,available,on_sale FROM products WHERE id=?", productID).Scan(&product.ID, &product.Name, &product.Price, &product.Available, &product.OnSale)
	if err == sql.ErrNoRows {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, err
	}
	if !product.OnSale || product.Available < quantity {
		return Order{}, ErrOutOfStock
	}
	result, err := tx.ExecContext(ctx, "UPDATE products SET available=available-? WHERE id=? AND on_sale=1 AND available>=?", quantity, productID, quantity)
	if err != nil {
		return Order{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return Order{}, err
	}
	if changed != 1 {
		return Order{}, ErrOutOfStock
	}

	now := s.now()
	order := Order{ID: newID("ORD", now), BuyerID: buyer.ID, BuyerName: buyer.Name, ProductID: product.ID, ProductName: product.Name, UnitPrice: product.Price, Quantity: quantity, Total: product.Price * int64(quantity), Status: OrderPending, CreatedAt: now, CloseAt: now.Add(10 * time.Minute)}
	_, err = tx.ExecContext(ctx, `INSERT INTO orders(id,buyer_id,product_id,product_name,unit_price,quantity,total_cents,status,created_at,close_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, order.ID, order.BuyerID, order.ProductID, order.ProductName, order.UnitPrice, order.Quantity, order.Total, order.Status, formatTime(order.CreatedAt), formatTime(order.CloseAt))
	if err != nil {
		return Order{}, err
	}
	if err := s.audit(ctx, tx, buyer.Name, "创建订单", fmt.Sprintf("%s 占用 %d 件 %s", order.ID, quantity, product.Name)); err != nil {
		return Order{}, err
	}
	if err := tx.Commit(); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (s *Store) CancelOrder(ctx context.Context, buyer User, orderID, reason string) error {
	if buyer.Role != RoleBuyer {
		return ErrForbidden
	}
	return s.closeOrder(ctx, orderID, buyer.ID, buyer.Name, reason)
}

func (s *Store) ExpirePending(ctx context.Context) (int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM orders WHERE status=? AND close_at<?", OrderPending, formatTime(s.now()))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		if err := s.closeOrder(ctx, id, "", "系统", "超时关闭"); err != nil {
			return 0, err
		}
	}
	return len(ids), rows.Err()
}

func (s *Store) closeOrder(ctx context.Context, orderID, buyerID, actor, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var productID string
	var quantity int
	var ownerID string
	err = tx.QueryRowContext(ctx, "SELECT product_id,quantity,buyer_id FROM orders WHERE id=? AND status=?", orderID, OrderPending).Scan(&productID, &quantity, &ownerID)
	if err == sql.ErrNoRows {
		return ErrInvalidState
	}
	if err != nil {
		return err
	}
	if buyerID != "" && ownerID != buyerID {
		return ErrForbidden
	}
	result, err := tx.ExecContext(ctx, "UPDATE orders SET status=?,closed_at=?,close_reason=? WHERE id=? AND status=?", OrderClosed, formatTime(s.now()), reason, orderID, OrderPending)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrInvalidState
	}
	if _, err = tx.ExecContext(ctx, "UPDATE products SET available=available+? WHERE id=?", quantity, productID); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, actor, reason, fmt.Sprintf("%s 释放 %d 件库存", orderID, quantity)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Ship(ctx context.Context, operator User, orderID, carrier, tracking string) error {
	if operator.Role != RoleWarehouse {
		return ErrForbidden
	}
	if carrier == "" || tracking == "" {
		return ErrNotFound
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE orders SET status=? WHERE id=? AND status=?", OrderShipped, orderID, OrderAwaitShipment)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrInvalidState
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO shipments(order_id,carrier,tracking,operator_id,created_at) VALUES(?,?,?,?,?)", orderID, carrier, tracking, operator.ID, formatTime(s.now()))
	if err != nil {
		return err
	}
	if err := s.audit(ctx, tx, operator.Name, "登记发货", fmt.Sprintf("%s %s %s", orderID, carrier, tracking)); err != nil {
		return err
	}
	return tx.Commit()
}
