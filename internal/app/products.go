package app

import (
	"context"
	"fmt"
)

// UpdateProduct updates the current sellable stock and price without changing order snapshots.
func (s *Store) UpdateProduct(ctx context.Context, operator User, productID string, price int64, available int, onSale bool) error {
	if operator.Role != RoleOperations {
		return ErrForbidden
	}
	if productID == "" || price < 0 || available < 0 {
		return ErrInvalidState
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE products SET price_cents=?,available=?,on_sale=? WHERE id=?", price, available, boolInt(onSale), productID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrNotFound
	}
	if err := s.audit(ctx, tx, operator.Name, "维护商品", fmt.Sprintf("%s 售价 %d 分，可售 %d，上架=%t", productID, price, available, onSale)); err != nil {
		return err
	}
	return tx.Commit()
}
