package app

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func (s *Store) RequestRefund(ctx context.Context, buyer User, orderID string) error {
	if buyer.Role != RoleBuyer {
		return ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var total int64
	var ownerID string
	err = tx.QueryRowContext(ctx, "SELECT total_cents,buyer_id FROM orders WHERE id=? AND status=?", orderID, OrderAwaitShipment).Scan(&total, &ownerID)
	if err == sql.ErrNoRows {
		return ErrInvalidState
	}
	if err != nil {
		return err
	}
	if ownerID != buyer.ID {
		return ErrForbidden
	}
	result, err := tx.ExecContext(ctx, "UPDATE orders SET status=? WHERE id=? AND status=?", OrderRefundReview, orderID, OrderAwaitShipment)
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
	now := s.now()
	_, err = tx.ExecContext(ctx, "INSERT INTO refunds(id,order_id,kind,amount_cents,status,requested_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", newID("RF", now), orderID, "NORMAL", total, RefundReview, buyer.ID, formatTime(now), formatTime(now))
	if err != nil {
		return err
	}
	if err := s.audit(ctx, tx, buyer.Name, "申请售前退款", fmt.Sprintf("%s 已暂停发货", orderID)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ReviewRefund(ctx context.Context, operator User, refundID, reason string, approved bool) error {
	if operator.Role != RoleService {
		return ErrForbidden
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrReasonRequired
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var orderID string
	err = tx.QueryRowContext(ctx, "SELECT order_id FROM refunds WHERE id=? AND status=?", refundID, RefundReview).Scan(&orderID)
	if err == sql.ErrNoRows {
		return ErrInvalidState
	}
	if err != nil {
		return err
	}
	now := formatTime(s.now())
	if approved {
		if _, err = tx.ExecContext(ctx, "UPDATE refunds SET status=?,reason=?,updated_at=? WHERE id=?", RefundPendingChannel, reason, now, refundID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE orders SET status=? WHERE id=? AND status=?", OrderRefunding, orderID, OrderRefundReview); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, operator.Name, "同意退款", fmt.Sprintf("%s：%s", orderID, reason)); err != nil {
			return err
		}
	} else {
		if _, err = tx.ExecContext(ctx, "UPDATE refunds SET status=?,reason=?,updated_at=? WHERE id=?", RefundRejected, reason, now, refundID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE orders SET status=? WHERE id=? AND status=?", OrderAwaitShipment, orderID, OrderRefundReview); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, operator.Name, "拒绝退款", fmt.Sprintf("%s：%s", orderID, reason)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) RetryRefund(ctx context.Context, operator User, refundID, reason string) error {
	if operator.Role != RoleService {
		return ErrForbidden
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ErrReasonRequired
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE refunds SET status=?,reason=?,updated_at=? WHERE id=? AND status=?", RefundPendingChannel, "客服重试："+reason, formatTime(s.now()), refundID, RefundFailed)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return ErrInvalidState
	}
	if err := s.audit(ctx, tx, operator.Name, "发起退款重试", fmt.Sprintf("%s：%s", refundID, reason)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) StartLateRefund(ctx context.Context, operator User, exceptionID string) error {
	if operator.Role != RoleService {
		return ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var amount int64
	err = tx.QueryRowContext(ctx, "SELECT amount_cents FROM exceptions WHERE id=? AND status=?", exceptionID, "PENDING_REFUND").Scan(&amount)
	if err == sql.ErrNoRows {
		return ErrInvalidState
	}
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "UPDATE exceptions SET status=? WHERE id=? AND status=?", "REFUNDING", exceptionID, "PENDING_REFUND")
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
	now := s.now()
	_, err = tx.ExecContext(ctx, "INSERT INTO refunds(id,exception_id,kind,amount_cents,status,requested_by,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", newID("RF", now), exceptionID, "LATE", amount, RefundPendingChannel, operator.ID, formatTime(now), formatTime(now))
	if err != nil {
		return err
	}
	if err := s.audit(ctx, tx, operator.Name, "发起迟到支付退款", exceptionID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RecordRefund(ctx context.Context, channel User, refundID string, success bool) error {
	if channel.Role != RoleChannel {
		return ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var refund Refund
	var orderID, exceptionID sql.NullString
	err = tx.QueryRowContext(ctx, "SELECT id,order_id,exception_id,kind,amount_cents,status FROM refunds WHERE id=?", refundID).Scan(&refund.ID, &orderID, &exceptionID, &refund.Kind, &refund.Amount, &refund.Status)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if orderID.Valid {
		refund.OrderID = orderID.String
	}
	if exceptionID.Valid {
		refund.ExceptionID = exceptionID.String
	}
	if refund.Status == RefundSuccess {
		return ErrDuplicate
	}
	if refund.Status != RefundPendingChannel {
		return ErrInvalidState
	}
	if !success {
		return s.failRefund(ctx, tx, channel, refund)
	}
	if refund.Kind == "NORMAL" {
		return s.completeNormalRefund(ctx, tx, channel, refund)
	}
	return s.completeLateRefund(ctx, tx, channel, refund)
}

func (s *Store) failRefund(ctx context.Context, tx *sql.Tx, channel User, refund Refund) error {
	_, err := tx.ExecContext(ctx, "UPDATE refunds SET status=?,reason=?,updated_at=? WHERE id=?", RefundFailed, "模拟渠道退款失败", formatTime(s.now()), refund.ID)
	if err != nil {
		return err
	}
	if err := s.audit(ctx, tx, channel.Name, "退款渠道失败", refund.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) completeNormalRefund(ctx context.Context, tx *sql.Tx, channel User, refund Refund) error {
	var productID string
	var quantity int
	var restored bool
	err := tx.QueryRowContext(ctx, "SELECT product_id,quantity,stock_restored FROM orders WHERE id=? AND status=?", refund.OrderID, OrderRefunding).Scan(&productID, &quantity, &restored)
	if err != nil {
		return ErrInvalidState
	}
	if _, err = tx.ExecContext(ctx, "UPDATE refunds SET status=?,updated_at=? WHERE id=?", RefundSuccess, formatTime(s.now()), refund.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE orders SET status=? WHERE id=?", OrderRefunded, refund.OrderID); err != nil {
		return err
	}
	if !restored {
		if _, err = tx.ExecContext(ctx, "UPDATE products SET available=available+? WHERE id=?", quantity, productID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE orders SET stock_restored=1 WHERE id=?", refund.OrderID); err != nil {
			return err
		}
	}
	if err := s.audit(ctx, tx, channel.Name, "退款成功", fmt.Sprintf("%s 已恢复库存", refund.OrderID)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) completeLateRefund(ctx context.Context, tx *sql.Tx, channel User, refund Refund) error {
	if _, err := tx.ExecContext(ctx, "UPDATE refunds SET status=?,updated_at=? WHERE id=?", RefundSuccess, formatTime(s.now()), refund.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE exceptions SET status=? WHERE id=?", "REFUNDED", refund.ExceptionID); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, channel.Name, "迟到支付退款成功", fmt.Sprintf("%s 不恢复库存", refund.ExceptionID)); err != nil {
		return err
	}
	return tx.Commit()
}
