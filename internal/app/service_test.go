package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "shop.db"))
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func mustUser(t *testing.T, store *Store, id string) User {
	t.Helper()
	user, err := store.User(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func available(t *testing.T, store *Store, productID string) int {
	t.Helper()
	products, err := store.Products(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, product := range products {
		if product.ID == productID {
			return product.Available
		}
	}
	t.Fatalf("product %s not found", productID)
	return 0
}

func TestOrderCancelReleasesInventoryOnce(t *testing.T) {
	store := testStore(t)
	buyer := mustUser(t, store, "buyer-1")
	order, err := store.CreateOrder(context.Background(), buyer, "p1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := available(t, store, "p1"); got != 3 {
		t.Fatalf("available = %d, want 3", got)
	}
	if err := store.CancelOrder(context.Background(), buyer, order.ID, "买家取消"); err != nil {
		t.Fatal(err)
	}
	if got := available(t, store, "p1"); got != 5 {
		t.Fatalf("available = %d, want 5", got)
	}
	if err := store.CancelOrder(context.Background(), buyer, order.ID, "买家取消"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("second cancel error = %v", err)
	}
	if got := available(t, store, "p1"); got != 5 {
		t.Fatalf("available changed twice = %d", got)
	}
}

func TestPaymentDuplicateIsIdempotent(t *testing.T) {
	store := testStore(t)
	buyer := mustUser(t, store, "buyer-1")
	channel := mustUser(t, store, "channel-1")
	order, err := store.CreateOrder(context.Background(), buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(context.Background(), channel, order.ID, "PAY-1", order.Total, true); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(context.Background(), channel, order.ID, "PAY-1", order.Total, true); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate error = %v", err)
	}
	orders, err := store.Orders(context.Background(), buyer)
	if err != nil {
		t.Fatal(err)
	}
	if orders[0].Status != OrderAwaitShipment {
		t.Fatalf("status = %s", orders[0].Status)
	}
}

func TestLatePaymentCreatesExceptionWithoutInventoryRecovery(t *testing.T) {
	store := testStore(t)
	buyer := mustUser(t, store, "buyer-1")
	channel := mustUser(t, store, "channel-1")
	order, err := store.CreateOrder(context.Background(), buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CancelOrder(context.Background(), buyer, order.ID, "超时关闭"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(context.Background(), channel, order.ID, "PAY-LATE", order.Total, true); err != nil {
		t.Fatal(err)
	}
	if got := available(t, store, "p1"); got != 5 {
		t.Fatalf("late payment changed stock: %d", got)
	}
	exceptions, err := store.Exceptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(exceptions) != 1 || exceptions[0].Status != "PENDING_REFUND" {
		t.Fatalf("exceptions = %#v", exceptions)
	}
}

func TestExpiredPendingPaymentBecomesLateException(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	buyer := mustUser(t, store, "buyer-1")
	channel := mustUser(t, store, "channel-1")
	order, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return order.CloseAt }
	if err := store.RecordPayment(ctx, channel, order.ID, "PAY-EXPIRED", order.Total, true); err != nil {
		t.Fatal(err)
	}
	orders, err := store.Orders(ctx, buyer)
	if err != nil || orders[0].Status != OrderClosed {
		t.Fatalf("expired payment order=%#v, err=%v", orders, err)
	}
	if got := available(t, store, "p1"); got != 5 {
		t.Fatalf("expired payment stock=%d, want 5", got)
	}
	exceptions, err := store.Exceptions(ctx)
	if err != nil || len(exceptions) != 1 || exceptions[0].Status != "PENDING_REFUND" {
		t.Fatalf("expired payment exceptions=%#v, err=%v", exceptions, err)
	}
}

func TestRefundReviewRequiresReason(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	buyer := mustUser(t, store, "buyer-1")
	channel := mustUser(t, store, "channel-1")
	service := mustUser(t, store, "service-1")
	order, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(ctx, channel, order.ID, "PAY-REASON", order.Total, true); err != nil {
		t.Fatal(err)
	}
	if err := store.RequestRefund(ctx, buyer, order.ID); err != nil {
		t.Fatal(err)
	}
	refunds, err := store.Refunds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReviewRefund(ctx, service, refunds[0].ID, " ", true); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("blank review reason error=%v", err)
	}
	refunds, err = store.Refunds(ctx)
	if err != nil || refunds[0].Status != RefundReview {
		t.Fatalf("blank review changed refund=%#v, err=%v", refunds, err)
	}
}

func TestPaymentAndShipmentRecordsAreQueryable(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	buyer := mustUser(t, store, "buyer-1")
	channel := mustUser(t, store, "channel-1")
	warehouse := mustUser(t, store, "warehouse-1")
	operator := mustUser(t, store, "ops-1")
	order, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(ctx, channel, order.ID, "PAY-QUERY", order.Total, true); err != nil {
		t.Fatal(err)
	}
	if err := store.Ship(ctx, warehouse, order.ID, "顺丰", "SF-QUERY-1"); err != nil {
		t.Fatal(err)
	}
	orders, err := store.Orders(ctx, buyer)
	if err != nil || orders[0].Tracking != "SF-QUERY-1" {
		t.Fatalf("tracking order=%#v, err=%v", orders, err)
	}
	dashboard, err := store.Dashboard(ctx, operator)
	if err != nil || len(dashboard.Payments) != 1 || len(dashboard.Shipments) != 1 {
		t.Fatalf("operations records=%#v, err=%v", dashboard, err)
	}
}

func TestRefundSuccessRestoresStockOnce(t *testing.T) {
	store := testStore(t)
	buyer := mustUser(t, store, "buyer-1")
	channel := mustUser(t, store, "channel-1")
	service := mustUser(t, store, "service-1")
	order, err := store.CreateOrder(context.Background(), buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(context.Background(), channel, order.ID, "PAY-2", order.Total, true); err != nil {
		t.Fatal(err)
	}
	if err := store.RequestRefund(context.Background(), buyer, order.ID); err != nil {
		t.Fatal(err)
	}
	refunds, err := store.Refunds(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReviewRefund(context.Background(), service, refunds[0].ID, "售前退款", true); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRefund(context.Background(), channel, refunds[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if got := available(t, store, "p1"); got != 5 {
		t.Fatalf("available = %d, want 5", got)
	}
	if err := store.RecordRefund(context.Background(), channel, refunds[0].ID, true); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate refund error = %v", err)
	}
	if got := available(t, store, "p1"); got != 5 {
		t.Fatalf("stock restored twice = %d", got)
	}
}

func TestConcurrentOrdersDoNotOversell(t *testing.T) {
	store := testStore(t)
	buyer := mustUser(t, store, "buyer-1")
	var group sync.WaitGroup
	results := make(chan error, 20)
	for range 20 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := store.CreateOrder(context.Background(), buyer, "p1", 1)
			results <- err
		}()
	}
	group.Wait()
	close(results)
	created := 0
	for err := range results {
		if err == nil {
			created++
			continue
		}
		if !errors.Is(err, ErrOutOfStock) {
			t.Fatalf("unexpected concurrent order error: %v", err)
		}
	}
	if created != 5 {
		t.Fatalf("created = %d, want 5", created)
	}
	if got := available(t, store, "p1"); got != 0 {
		t.Fatalf("available = %d, want 0", got)
	}
}

func TestRefundRequestAndShipmentHaveOneLegalWinner(t *testing.T) {
	store := testStore(t)
	buyer := mustUser(t, store, "buyer-1")
	channel := mustUser(t, store, "channel-1")
	warehouse := mustUser(t, store, "warehouse-1")
	order, err := store.CreateOrder(context.Background(), buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(context.Background(), channel, order.ID, "PAY-RACE", order.Total, true); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	go func() { results <- store.RequestRefund(context.Background(), buyer, order.ID) }()
	go func() { results <- store.Ship(context.Background(), warehouse, order.ID, "顺丰速运", "SF001") }()
	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatalf("expected exactly one winner, got %v and %v", first, second)
	}
	orders, err := store.Orders(context.Background(), buyer)
	if err != nil {
		t.Fatal(err)
	}
	if orders[0].Status != OrderRefundReview && orders[0].Status != OrderShipped {
		t.Fatalf("illegal race result: %s", orders[0].Status)
	}
}

func TestPriceSnapshotSurvivesOperationsUpdate(t *testing.T) {
	store := testStore(t)
	buyer := mustUser(t, store, "buyer-1")
	operator := mustUser(t, store, "ops-1")
	channel := mustUser(t, store, "channel-1")
	order, err := store.CreateOrder(context.Background(), buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateProduct(context.Background(), operator, "p1", 12000, 4, true); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(context.Background(), channel, order.ID, "PAY-SNAPSHOT", 10000, true); err != nil {
		t.Fatalf("payment should use price snapshot: %v", err)
	}
	orders, err := store.Orders(context.Background(), buyer)
	if err != nil {
		t.Fatal(err)
	}
	if orders[0].Total != 10000 || orders[0].UnitPrice != 10000 {
		t.Fatalf("order snapshot changed: %#v", orders[0])
	}
}

func TestValidationTimeoutAndOperationalQueries(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	buyer := mustUser(t, store, "buyer-1")
	operator := mustUser(t, store, "ops-1")
	channel := mustUser(t, store, "channel-1")
	warehouse := mustUser(t, store, "warehouse-1")
	service := mustUser(t, store, "service-1")

	if _, err := store.CreateOrder(ctx, buyer, "p1", 0); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("invalid quantity error = %v", err)
	}
	if _, err := store.CreateOrder(ctx, operator, "p1", 1); !errors.Is(err, ErrForbidden) {
		t.Fatalf("role error = %v", err)
	}
	if err := store.UpdateProduct(ctx, buyer, "p1", 10000, 5, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("update role error = %v", err)
	}
	if err := store.UpdateProduct(ctx, operator, "missing", 10000, 1, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing product error = %v", err)
	}

	order, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 11, 13, 10, 11, 0, 0, time.UTC) }
	if count, err := store.ExpirePending(ctx); err != nil || count != 1 {
		t.Fatalf("expire = %d, %v", count, err)
	}
	if _, err := store.ExpirePending(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(ctx, channel, order.ID, "PAY-CLOSED", order.Total, true); err != nil {
		t.Fatal(err)
	}
	exceptions, err := store.Exceptions(ctx)
	if err != nil || len(exceptions) != 1 {
		t.Fatalf("exceptions = %#v, %v", exceptions, err)
	}
	if err := store.StartLateRefund(ctx, service, exceptions[0].ID); err != nil {
		t.Fatal(err)
	}
	refunds, err := store.Refunds(ctx)
	if err != nil || len(refunds) != 1 {
		t.Fatalf("refunds = %#v, %v", refunds, err)
	}
	if err := store.RecordRefund(ctx, channel, refunds[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if got := available(t, store, "p1"); got != 5 {
		t.Fatalf("late refund restored stock: %d", got)
	}

	if _, err := store.Products(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Orders(ctx, operator); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Shipments(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Audits(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Summary(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Dashboard(ctx, operator); err != nil {
		t.Fatal(err)
	}
	_ = warehouse
}

func TestPaymentValidationAndRefundRetry(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	buyer := mustUser(t, store, "buyer-1")
	channel := mustUser(t, store, "channel-1")
	service := mustUser(t, store, "service-1")
	order, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(ctx, channel, order.ID, "PAY-BAD-SIGN", order.Total, false); err != nil {
		t.Fatalf("invalid signature should be recorded: %v", err)
	}
	if err := store.RecordPayment(ctx, channel, order.ID, "PAY-BAD-AMOUNT", 1, true); !errors.Is(err, ErrAmountMismatch) {
		t.Fatalf("amount error = %v", err)
	}
	if err := store.RecordPayment(ctx, channel, "missing", "PAY-UNKNOWN", order.Total, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown order error = %v", err)
	}
	if err := store.RecordPayment(ctx, channel, order.ID, "PAY-VALID", order.Total, true); err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(ctx, channel, other.ID, "PAY-VALID", other.Total, true); !errors.Is(err, ErrTxConflict) {
		t.Fatalf("transaction conflict error = %v", err)
	}
	if err := store.RequestRefund(ctx, buyer, order.ID); err != nil {
		t.Fatal(err)
	}
	refunds, err := store.Refunds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReviewRefund(ctx, service, refunds[0].ID, "客户确认", true); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRefund(ctx, channel, refunds[0].ID, false); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRefund(ctx, channel, refunds[0].ID, true); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("channel must wait for service retry: %v", err)
	}
	if err := store.RetryRefund(ctx, service, refunds[0].ID, "渠道恢复"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRefund(ctx, channel, refunds[0].ID, true); err != nil {
		t.Fatal(err)
	}
}

func TestRefundRejectedAllowsShipment(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	buyer := mustUser(t, store, "buyer-1")
	channel := mustUser(t, store, "channel-1")
	service := mustUser(t, store, "service-1")
	warehouse := mustUser(t, store, "warehouse-1")
	order, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(ctx, channel, order.ID, "PAY-REJECT", order.Total, true); err != nil {
		t.Fatal(err)
	}
	if err := store.RequestRefund(ctx, buyer, order.ID); err != nil {
		t.Fatal(err)
	}
	refunds, err := store.Refunds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReviewRefund(ctx, service, refunds[0].ID, "已进入发货计划", false); err != nil {
		t.Fatal(err)
	}
	if err := store.Ship(ctx, warehouse, order.ID, "中通", "ZT001"); err != nil {
		t.Fatal(err)
	}
	if err := store.Ship(ctx, warehouse, order.ID, "中通", "ZT002"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("duplicate shipment error = %v", err)
	}
}

func TestPermissionAndInvalidStateBoundaries(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	buyer := mustUser(t, store, "buyer-1")
	otherBuyer := mustUser(t, store, "buyer-2")
	operator := mustUser(t, store, "ops-1")
	warehouse := mustUser(t, store, "warehouse-1")
	service := mustUser(t, store, "service-1")
	channel := mustUser(t, store, "channel-1")
	if _, err := store.User(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user error = %v", err)
	}
	if _, err := store.CreateOrder(ctx, buyer, "missing", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing product error = %v", err)
	}
	if err := store.UpdateProduct(ctx, operator, "p1", -1, 1, true); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("invalid price error = %v", err)
	}
	if err := store.UpdateProduct(ctx, operator, "p1", 10000, -1, true); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("invalid stock error = %v", err)
	}
	if err := store.RecordPayment(ctx, buyer, "missing", "PAY-NO-ROLE", 1, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("payment role error = %v", err)
	}
	if err := store.RecordPayment(ctx, channel, "missing", "", 1, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty tx error = %v", err)
	}
	order, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CancelOrder(ctx, otherBuyer, order.ID, "越权"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cancel ownership error = %v", err)
	}
	if err := store.CancelOrder(ctx, operator, order.ID, "越权"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cancel role error = %v", err)
	}
	if err := store.CancelOrder(ctx, buyer, "missing", "不存在"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("cancel missing error = %v", err)
	}
	if err := store.RequestRefund(ctx, buyer, order.ID); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("pending refund error = %v", err)
	}
	if err := store.Ship(ctx, buyer, order.ID, "", ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ship role error = %v", err)
	}
	if err := store.Ship(ctx, warehouse, order.ID, "", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ship fields error = %v", err)
	}
	if err := store.ReviewRefund(ctx, buyer, "missing", "越权", true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("review role error = %v", err)
	}
	if err := store.ReviewRefund(ctx, service, "missing", "不存在", true); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("review missing error = %v", err)
	}
	if err := store.StartLateRefund(ctx, buyer, "missing"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("late refund role error = %v", err)
	}
	if err := store.RecordRefund(ctx, buyer, "missing", true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("refund channel role error = %v", err)
	}
	if err := store.RecordRefund(ctx, channel, "missing", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("refund missing error = %v", err)
	}
	if _, err := store.Dashboard(ctx, buyer); err != nil {
		t.Fatal(err)
	}
}

func TestStateRejectionMatrixAndRepeatedChannelFailures(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	buyer := mustUser(t, store, "buyer-1")
	otherBuyer := mustUser(t, store, "buyer-2")
	operator := mustUser(t, store, "ops-1")
	warehouse := mustUser(t, store, "warehouse-1")
	service := mustUser(t, store, "service-1")
	channel := mustUser(t, store, "channel-1")

	if _, err := store.CreateOrder(ctx, buyer, "p1", 3); !errors.Is(err, ErrInvalidQuantity) {
		t.Fatalf("quantity above limit = %v", err)
	}
	if err := store.UpdateProduct(ctx, operator, "p1", 10000, 5, false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOrder(ctx, buyer, "p1", 1); !errors.Is(err, ErrOutOfStock) {
		t.Fatalf("off-sale order = %v", err)
	}
	if err := store.UpdateProduct(ctx, operator, "p1", 10000, 0, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateOrder(ctx, buyer, "p1", 1); !errors.Is(err, ErrOutOfStock) {
		t.Fatalf("empty-stock order = %v", err)
	}
	if err := store.UpdateProduct(ctx, operator, "p1", 10000, 5, true); err != nil {
		t.Fatal(err)
	}

	order, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RequestRefund(ctx, otherBuyer, order.ID); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("refund ownership = %v", err)
	}
	if err := store.RecordPayment(ctx, channel, order.ID, "PAY-MATRIX", order.Total, true); err != nil {
		t.Fatal(err)
	}
	if err := store.RequestRefund(ctx, buyer, order.ID); err != nil {
		t.Fatal(err)
	}
	refunds, err := store.Refunds(ctx)
	if err != nil || len(refunds) != 1 {
		t.Fatalf("matrix refunds = %#v, %v", refunds, err)
	}
	if err := store.RecordRefund(ctx, channel, refunds[0].ID, true); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("refund before review = %v", err)
	}
	if err := store.ReviewRefund(ctx, service, refunds[0].ID, "处理中", true); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRefund(ctx, channel, refunds[0].ID, false); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRefund(ctx, channel, refunds[0].ID, false); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("channel cannot retry failed refund: %v", err)
	}
	if err := store.RetryRefund(ctx, service, refunds[0].ID, "客服复核"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRefund(ctx, channel, refunds[0].ID, false); err != nil {
		t.Fatal(err)
	}

	shipped, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(ctx, channel, shipped.ID, "PAY-SHIPPED", shipped.Total, true); err != nil {
		t.Fatal(err)
	}
	if err := store.Ship(ctx, warehouse, shipped.ID, "圆通", "YT001"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(ctx, channel, shipped.ID, "PAY-SHIPPED-2", shipped.Total, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("payment after shipment = %v", err)
	}
	if err := store.RequestRefund(ctx, buyer, shipped.ID); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("refund after shipment = %v", err)
	}
	if err := store.Ship(ctx, warehouse, "missing", "圆通", "YT002"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("missing shipment = %v", err)
	}
}

func TestOpenExistingDatabaseAndPaymentAfterInvalidState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reopen.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	buyer := mustUser(t, second, "buyer-1")
	channel := mustUser(t, second, "channel-1")
	order, err := second.CreateOrder(context.Background(), buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.CancelOrder(context.Background(), buyer, order.ID, "测试关闭"); err != nil {
		t.Fatal(err)
	}
	if err := second.RecordPayment(context.Background(), channel, order.ID, "PAY-REOPEN", order.Total, true); err != nil {
		t.Fatal(err)
	}
}

func TestClosedStorePropagatesDatabaseErrors(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	buyer := mustUser(t, store, "buyer-1")
	operator := mustUser(t, store, "ops-1")
	warehouse := mustUser(t, store, "warehouse-1")
	service := mustUser(t, store, "service-1")
	channel := mustUser(t, store, "channel-1")
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.User(ctx, buyer.ID); err == nil {
		t.Fatal("User should return a closed database error")
	}
	if _, err := store.Products(ctx); err == nil {
		t.Fatal("Products should return a closed database error")
	}
	if _, err := store.Orders(ctx, buyer); err == nil {
		t.Fatal("Orders should return a closed database error")
	}
	if _, err := store.Refunds(ctx); err == nil {
		t.Fatal("Refunds should return a closed database error")
	}
	if _, err := store.Exceptions(ctx); err == nil {
		t.Fatal("Exceptions should return a closed database error")
	}
	if _, err := store.Shipments(ctx); err == nil {
		t.Fatal("Shipments should return a closed database error")
	}
	if _, err := store.Audits(ctx); err == nil {
		t.Fatal("Audits should return a closed database error")
	}
	if _, err := store.Summary(ctx); err == nil {
		t.Fatal("Summary should return a closed database error")
	}
	if _, err := store.Dashboard(ctx, operator); err == nil {
		t.Fatal("Dashboard should return a closed database error")
	}
	if _, err := store.CreateOrder(ctx, buyer, "p1", 1); err == nil {
		t.Fatal("CreateOrder should return a closed database error")
	}
	if err := store.CancelOrder(ctx, buyer, "missing", "关闭"); err == nil {
		t.Fatal("CancelOrder should return a closed database error")
	}
	if _, err := store.ExpirePending(ctx); err == nil {
		t.Fatal("ExpirePending should return a closed database error")
	}
	if err := store.Ship(ctx, warehouse, "missing", "快递", "TRACK"); err == nil {
		t.Fatal("Ship should return a closed database error")
	}
	if err := store.UpdateProduct(ctx, operator, "p1", 1, 1, true); err == nil {
		t.Fatal("UpdateProduct should return a closed database error")
	}
	if err := store.RecordPayment(ctx, channel, "missing", "PAY-CLOSED-DB", 1, true); err == nil {
		t.Fatal("RecordPayment should return a closed database error")
	}
	if err := store.RequestRefund(ctx, buyer, "missing"); err == nil {
		t.Fatal("RequestRefund should return a closed database error")
	}
	if err := store.ReviewRefund(ctx, service, "missing", "关闭", true); err == nil {
		t.Fatal("ReviewRefund should return a closed database error")
	}
	if err := store.StartLateRefund(ctx, service, "missing"); err == nil {
		t.Fatal("StartLateRefund should return a closed database error")
	}
	if err := store.RecordRefund(ctx, channel, "missing", true); err == nil {
		t.Fatal("RecordRefund should return a closed database error")
	}
}

func TestQueriesRejectMalformedPersistedTimes(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	buyer := mustUser(t, store, "buyer-1")
	channel := mustUser(t, store, "channel-1")
	order, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE orders SET created_at='bad-time' WHERE id=?", order.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Orders(ctx, buyer); err == nil {
		t.Fatal("Orders should reject malformed created_at")
	}
	if err := store.RecordPayment(ctx, channel, order.ID, "PAY-MALFORMED", order.Total, true); err != nil {
		t.Fatal(err)
	}
	refundOrder, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(ctx, channel, refundOrder.ID, "PAY-REFUND-TIME", refundOrder.Total, true); err != nil {
		t.Fatal(err)
	}
	if err := store.RequestRefund(ctx, buyer, refundOrder.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE refunds SET created_at='bad-time' WHERE order_id=?", refundOrder.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Refunds(ctx); err == nil {
		t.Fatal("Refunds should reject malformed created_at")
	}
	if _, err := store.db.ExecContext(ctx, "INSERT INTO shipments(order_id,carrier,tracking,operator_id,created_at) VALUES(?,?,?,?,?)", "ORD-BAD", "快递", "BAD", "warehouse-1", "bad-time"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Shipments(ctx); err == nil {
		t.Fatal("Shipments should reject malformed created_at")
	}
	if _, err := store.db.ExecContext(ctx, "INSERT INTO exceptions(id,order_id,channel_tx,amount_cents,kind,status,reason,created_at) VALUES(?,?,?,?,?,?,?,?)", "EXC-BAD", "ORD-BAD", "PAY-BAD-TIME", 1, "TEST", "PENDING_REFUND", "bad", "bad-time"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exceptions(ctx); err == nil {
		t.Fatal("Exceptions should reject malformed created_at")
	}
	if _, err := store.db.ExecContext(ctx, "INSERT INTO audits(created_at,actor,action,detail) VALUES(?,?,?,?)", "bad-time", "test", "bad", "bad"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Audits(ctx); err == nil {
		t.Fatal("Audits should reject malformed created_at")
	}
}

func TestSummaryAndDashboardPropagateLaterQueryFailures(t *testing.T) {
	ctx := context.Background()
	for _, table := range []string{"payments", "refunds", "orders"} {
		store := testStore(t)
		if _, err := store.db.ExecContext(ctx, "DROP TABLE "+table); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Summary(ctx); err == nil {
			t.Fatalf("Summary should fail after dropping %s", table)
		}
		_ = store.Close()
	}
	for _, table := range []string{"orders", "payments", "refunds", "shipments", "exceptions", "audits"} {
		store := testStore(t)
		operator := mustUser(t, store, "ops-1")
		if _, err := store.db.ExecContext(ctx, "DROP TABLE "+table); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Dashboard(ctx, operator); err == nil {
			t.Fatalf("Dashboard should fail after dropping %s", table)
		}
		_ = store.Close()
	}
}

func TestOrderScanRejectsMalformedOptionalTimes(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	buyer := mustUser(t, store, "buyer-1")
	channel := mustUser(t, store, "channel-1")
	order, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE orders SET close_at='bad-time' WHERE id=?", order.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Orders(ctx, buyer); err == nil {
		t.Fatal("Orders should reject malformed close_at")
	}
	paid, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(ctx, channel, paid.ID, "PAY-BAD-PAID-AT", paid.Total, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE orders SET paid_at='bad-time' WHERE id=?", paid.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Orders(ctx, buyer); err == nil {
		t.Fatal("Orders should reject malformed paid_at")
	}
}

func TestRefundCompletionRejectsChangedOrderState(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	buyer := mustUser(t, store, "buyer-1")
	channel := mustUser(t, store, "channel-1")
	service := mustUser(t, store, "service-1")
	order, err := store.CreateOrder(ctx, buyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPayment(ctx, channel, order.ID, "PAY-REFUND-STATE", order.Total, true); err != nil {
		t.Fatal(err)
	}
	if err := store.RequestRefund(ctx, buyer, order.ID); err != nil {
		t.Fatal(err)
	}
	refunds, err := store.Refunds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReviewRefund(ctx, service, refunds[0].ID, "审核通过", true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE orders SET status=? WHERE id=?", OrderShipped, order.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordRefund(ctx, channel, refunds[0].ID, true); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("changed order state refund = %v", err)
	}
}

func TestOpenRejectsDirectoryPath(t *testing.T) {
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("Open should reject a directory path")
	}
}

func TestAuditFailureRollsBackStateChanges(t *testing.T) {
	ctx := context.Background()
	newStore := func(t *testing.T) *Store {
		t.Helper()
		store := testStore(t)
		if _, err := store.db.ExecContext(ctx, "DROP TABLE audits"); err != nil {
			t.Fatal(err)
		}
		return store
	}

	createStore := newStore(t)
	buyer := mustUser(t, createStore, "buyer-1")
	if _, err := createStore.CreateOrder(ctx, buyer, "p1", 1); err == nil {
		t.Fatal("CreateOrder should fail when audit persistence fails")
	}
	if got := available(t, createStore, "p1"); got != 5 {
		t.Fatalf("CreateOrder rollback stock = %d", got)
	}

	updateStore := newStore(t)
	operator := mustUser(t, updateStore, "ops-1")
	if err := updateStore.UpdateProduct(ctx, operator, "p1", 12000, 4, true); err == nil {
		t.Fatal("UpdateProduct should fail when audit persistence fails")
	}

	cancelStore := testStore(t)
	cancelBuyer := mustUser(t, cancelStore, "buyer-1")
	cancelOrder, err := cancelStore.CreateOrder(ctx, cancelBuyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cancelStore.db.ExecContext(ctx, "DROP TABLE audits"); err != nil {
		t.Fatal(err)
	}
	if err := cancelStore.CancelOrder(ctx, cancelBuyer, cancelOrder.ID, "测试回滚"); err == nil {
		t.Fatal("CancelOrder should fail when audit persistence fails")
	}
	if got := available(t, cancelStore, "p1"); got != 4 {
		t.Fatalf("CancelOrder rollback stock = %d", got)
	}

	shipStore := testStore(t)
	shipBuyer := mustUser(t, shipStore, "buyer-1")
	shipChannel := mustUser(t, shipStore, "channel-1")
	shipWarehouse := mustUser(t, shipStore, "warehouse-1")
	shipOrder, err := shipStore.CreateOrder(ctx, shipBuyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := shipStore.RecordPayment(ctx, shipChannel, shipOrder.ID, "PAY-AUDIT-SHIP", shipOrder.Total, true); err != nil {
		t.Fatal(err)
	}
	if _, err := shipStore.db.ExecContext(ctx, "DROP TABLE audits"); err != nil {
		t.Fatal(err)
	}
	if err := shipStore.Ship(ctx, shipWarehouse, shipOrder.ID, "快递", "TRACK-AUDIT"); err == nil {
		t.Fatal("Ship should fail when audit persistence fails")
	}

	paymentStore := testStore(t)
	paymentBuyer := mustUser(t, paymentStore, "buyer-1")
	paymentChannel := mustUser(t, paymentStore, "channel-1")
	paymentOrder, err := paymentStore.CreateOrder(ctx, paymentBuyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := paymentStore.db.ExecContext(ctx, "DROP TABLE audits"); err != nil {
		t.Fatal(err)
	}
	if err := paymentStore.RecordPayment(ctx, paymentChannel, paymentOrder.ID, "PAY-AUDIT", paymentOrder.Total, true); err == nil {
		t.Fatal("RecordPayment should fail when audit persistence fails")
	}
}

func TestRefundAuditFailureRollsBackStateChanges(t *testing.T) {
	ctx := context.Background()
	setupPaidOrder := func(t *testing.T) (*Store, User, User, User, Order) {
		t.Helper()
		store := testStore(t)
		buyer := mustUser(t, store, "buyer-1")
		channel := mustUser(t, store, "channel-1")
		service := mustUser(t, store, "service-1")
		order, err := store.CreateOrder(ctx, buyer, "p1", 1)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.RecordPayment(ctx, channel, order.ID, "PAY-REFUND-AUDIT-"+order.ID, order.Total, true); err != nil {
			t.Fatal(err)
		}
		return store, buyer, channel, service, order
	}

	requestStore, buyer, _, _, requestOrder := setupPaidOrder(t)
	if _, err := requestStore.db.ExecContext(ctx, "DROP TABLE audits"); err != nil {
		t.Fatal(err)
	}
	if err := requestStore.RequestRefund(ctx, buyer, requestOrder.ID); err == nil {
		t.Fatal("RequestRefund should fail when audit persistence fails")
	}
	orders, err := requestStore.Orders(ctx, buyer)
	if err != nil || orders[0].Status != OrderAwaitShipment {
		t.Fatalf("request rollback orders=%#v, err=%v", orders, err)
	}

	reviewStore, reviewBuyer, _, service, reviewOrder := setupPaidOrder(t)
	if err := reviewStore.RequestRefund(ctx, reviewBuyer, reviewOrder.ID); err != nil {
		t.Fatal(err)
	}
	refunds, err := reviewStore.Refunds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviewStore.db.ExecContext(ctx, "DROP TABLE audits"); err != nil {
		t.Fatal(err)
	}
	if err := reviewStore.ReviewRefund(ctx, service, refunds[0].ID, "审核", true); err == nil {
		t.Fatal("ReviewRefund should fail when audit persistence fails")
	}

	completeStore, completeBuyer, completeChannel, completeService, completeOrder := setupPaidOrder(t)
	if err := completeStore.RequestRefund(ctx, completeBuyer, completeOrder.ID); err != nil {
		t.Fatal(err)
	}
	completeRefunds, err := completeStore.Refunds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := completeStore.ReviewRefund(ctx, completeService, completeRefunds[0].ID, "审核", true); err != nil {
		t.Fatal(err)
	}
	if _, err := completeStore.db.ExecContext(ctx, "DROP TABLE audits"); err != nil {
		t.Fatal(err)
	}
	if err := completeStore.RecordRefund(ctx, completeChannel, completeRefunds[0].ID, true); err == nil {
		t.Fatal("RecordRefund should fail when audit persistence fails")
	}
	if got := available(t, completeStore, "p1"); got != 4 {
		t.Fatalf("refund rollback stock = %d", got)
	}

	lateStore := testStore(t)
	lateBuyer := mustUser(t, lateStore, "buyer-1")
	lateChannel := mustUser(t, lateStore, "channel-1")
	lateService := mustUser(t, lateStore, "service-1")
	lateOrder, err := lateStore.CreateOrder(ctx, lateBuyer, "p1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := lateStore.CancelOrder(ctx, lateBuyer, lateOrder.ID, "关闭"); err != nil {
		t.Fatal(err)
	}
	if err := lateStore.RecordPayment(ctx, lateChannel, lateOrder.ID, "PAY-LATE-AUDIT", lateOrder.Total, true); err != nil {
		t.Fatal(err)
	}
	exceptions, err := lateStore.Exceptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lateStore.db.ExecContext(ctx, "DROP TABLE audits"); err != nil {
		t.Fatal(err)
	}
	if err := lateStore.StartLateRefund(ctx, lateService, exceptions[0].ID); err == nil {
		t.Fatal("StartLateRefund should fail when audit persistence fails")
	}

	rejectStore, rejectBuyer, _, rejectService, rejectOrder := setupPaidOrder(t)
	if err := rejectStore.RequestRefund(ctx, rejectBuyer, rejectOrder.ID); err != nil {
		t.Fatal(err)
	}
	rejectRefunds, err := rejectStore.Refunds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rejectStore.db.ExecContext(ctx, "DROP TABLE audits"); err != nil {
		t.Fatal(err)
	}
	if err := rejectStore.ReviewRefund(ctx, rejectService, rejectRefunds[0].ID, "拒绝", false); err == nil {
		t.Fatal("Rejected ReviewRefund should fail when audit persistence fails")
	}
}
