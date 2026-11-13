package app

import (
	"context"
	"database/sql"
	"fmt"
)

func (s *Store) RecordPayment(ctx context.Context, channel User, orderID, channelTx string, amount int64, validSignature bool) error {
	if channel.Role != RoleChannel {
		return ErrForbidden
	}
	if channelTx == "" {
		return ErrNotFound
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existingOrder sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT order_id FROM payments WHERE channel_tx=?", channelTx).Scan(&existingOrder)
	if err == nil {
		if existingOrder.Valid && existingOrder.String != orderID {
			return ErrTxConflict
		}
		return ErrDuplicate
	}
	if err != sql.ErrNoRows {
		return err
	}
	if !validSignature {
		_, err = tx.ExecContext(ctx, "INSERT INTO payments(id,channel_tx,amount_cents,verification_status,result,reason,received_at) VALUES(?,?,?,?,?,?,?)", newID("PAY", s.now()), channelTx, amount, "FAILED", "REJECTED", ErrInvalidSignature.Error(), formatTime(s.now()))
		if err != nil {
			return err
		}
		if err := s.audit(ctx, tx, channel.Name, "支付验签失败", channelTx); err != nil {
			return err
		}
		return tx.Commit()
	}

	var order Order
	var closeAt string
	err = tx.QueryRowContext(ctx, "SELECT id,buyer_id,product_id,product_name,unit_price,quantity,total_cents,status,close_at FROM orders WHERE id=?", orderID).Scan(&order.ID, &order.BuyerID, &order.ProductID, &order.ProductName, &order.UnitPrice, &order.Quantity, &order.Total, &order.Status, &closeAt)
	if err == sql.ErrNoRows {
		return s.recordInvalidPayment(ctx, tx, channel, channelTx, amount, "未知订单")
	}
	if err != nil {
		return err
	}
	if order.CloseAt, err = parseTime(closeAt); err != nil {
		return err
	}
	if amount != order.Total {
		return s.recordInvalidPayment(ctx, tx, channel, channelTx, amount, ErrAmountMismatch.Error())
	}
	if order.Status == OrderPending {
		if !s.now().Before(order.CloseAt) {
			return s.recordLatePayment(ctx, tx, channel, order, channelTx, true)
		}
		result, err := tx.ExecContext(ctx, "UPDATE orders SET status=?,payment_tx=?,paid_at=? WHERE id=? AND status=?", OrderAwaitShipment, channelTx, formatTime(s.now()), orderID, OrderPending)
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
		_, err = tx.ExecContext(ctx, "INSERT INTO payments(id,order_id,channel_tx,amount_cents,verification_status,result,received_at) VALUES(?,?,?,?,?,?,?)", newID("PAY", s.now()), orderID, channelTx, amount, "PASSED", "ACCEPTED", formatTime(s.now()))
		if err != nil {
			return err
		}
		if err := s.audit(ctx, tx, channel.Name, "支付成功通知", fmt.Sprintf("%s 转为待发货", orderID)); err != nil {
			return err
		}
		return tx.Commit()
	}
	if order.Status == OrderClosed {
		return s.recordLatePayment(ctx, tx, channel, order, channelTx, false)
	}
	return s.recordInvalidPayment(ctx, tx, channel, channelTx, amount, "订单当前状态不接受付款")
}

func (s *Store) recordLatePayment(ctx context.Context, tx *sql.Tx, channel User, order Order, channelTx string, closePending bool) error {
	if closePending {
		result, err := tx.ExecContext(ctx, "UPDATE orders SET status=?,closed_at=?,close_reason=? WHERE id=? AND status=?", OrderClosed, formatTime(s.now()), "支付通知到达时发现订单超时", order.ID, OrderPending)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			return ErrInvalidState
		}
		if _, err = tx.ExecContext(ctx, "UPDATE products SET available=available+? WHERE id=?", order.Quantity, order.ProductID); err != nil {
			return err
		}
	}
	now := s.now()
	if _, err := tx.ExecContext(ctx, "INSERT INTO payments(id,order_id,channel_tx,amount_cents,verification_status,result,reason,received_at) VALUES(?,?,?,?,?,?,?,?)", newID("PAY", now), order.ID, channelTx, order.Total, "PASSED", "LATE", "订单已关闭或超时", formatTime(now)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO exceptions(id,order_id,channel_tx,amount_cents,kind,status,reason,created_at) VALUES(?,?,?,?,?,?,?,?)", newID("EXC", now), order.ID, channelTx, order.Total, "LATE_PAYMENT", "PENDING_REFUND", "订单关闭后收到成功付款通知", formatTime(now)); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, channel.Name, "迟到支付异常", fmt.Sprintf("%s 未恢复订单或库存", order.ID)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) recordInvalidPayment(ctx context.Context, tx *sql.Tx, channel User, channelTx string, amount int64, reason string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO payments(id,channel_tx,amount_cents,verification_status,result,reason,received_at) VALUES(?,?,?,?,?,?,?)", newID("PAY", s.now()), channelTx, amount, "PASSED", "REJECTED", reason, formatTime(s.now()))
	if err != nil {
		return err
	}
	if err := s.audit(ctx, tx, channel.Name, "支付通知拒绝", fmt.Sprintf("%s：%s", channelTx, reason)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if reason == ErrAmountMismatch.Error() {
		return ErrAmountMismatch
	}
	return ErrNotFound
}
