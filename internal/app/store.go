package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, now: time.Now}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.seed(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	const schema = `
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, name TEXT NOT NULL, role TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS products (id TEXT PRIMARY KEY, name TEXT NOT NULL, price_cents INTEGER NOT NULL, available INTEGER NOT NULL, on_sale INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS orders (
 id TEXT PRIMARY KEY, buyer_id TEXT NOT NULL, product_id TEXT NOT NULL, product_name TEXT NOT NULL, unit_price INTEGER NOT NULL, quantity INTEGER NOT NULL, total_cents INTEGER NOT NULL,
 status TEXT NOT NULL, created_at TEXT NOT NULL, close_at TEXT NOT NULL, closed_at TEXT, close_reason TEXT, payment_tx TEXT UNIQUE, paid_at TEXT, stock_restored INTEGER NOT NULL DEFAULT 0,
 FOREIGN KEY(buyer_id) REFERENCES users(id));
CREATE TABLE IF NOT EXISTS payments (
 id TEXT PRIMARY KEY, order_id TEXT, channel_tx TEXT NOT NULL UNIQUE, amount_cents INTEGER NOT NULL, verification_status TEXT NOT NULL, result TEXT NOT NULL, reason TEXT, received_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS refunds (
 id TEXT PRIMARY KEY, order_id TEXT UNIQUE, exception_id TEXT UNIQUE, kind TEXT NOT NULL, amount_cents INTEGER NOT NULL, status TEXT NOT NULL, reason TEXT, requested_by TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, stock_restored INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS shipments (order_id TEXT PRIMARY KEY, carrier TEXT NOT NULL, tracking TEXT NOT NULL, operator_id TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS exceptions (id TEXT PRIMARY KEY, order_id TEXT NOT NULL, channel_tx TEXT NOT NULL UNIQUE, amount_cents INTEGER NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL, reason TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS audits (id INTEGER PRIMARY KEY AUTOINCREMENT, created_at TEXT NOT NULL, actor TEXT NOT NULL, action TEXT NOT NULL, detail TEXT NOT NULL);
`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

func (s *Store) seed(ctx context.Context) error {
	users := []User{{"buyer-1", "小聪", RoleBuyer}, {"buyer-2", "阿羽", RoleBuyer}, {"buyer-3", "陈星", RoleBuyer}, {"ops-1", "赵运营", RoleOperations}, {"warehouse-1", "仓库员", RoleWarehouse}, {"service-1", "客服员", RoleService}, {"channel-1", "模拟支付渠道", RoleChannel}}
	for _, user := range users {
		if _, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO users(id,name,role) VALUES(?,?,?)", user.ID, user.Name, user.Role); err != nil {
			return err
		}
	}
	products := []Product{{"p1", "保湿啫喱 500ml", 10000, 5, true}, {"p2", "手冲咖啡壶 600ml", 8800, 20, true}}
	for _, product := range products {
		if _, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO products(id,name,price_cents,available,on_sale) VALUES(?,?,?,?,?)", product.ID, product.Name, product.Price, product.Available, boolInt(product.OnSale)); err != nil {
			return err
		}
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func parseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func (s *Store) audit(ctx context.Context, tx *sql.Tx, actor, action, detail string) error {
	_, err := tx.ExecContext(ctx, "INSERT INTO audits(created_at,actor,action,detail) VALUES(?,?,?,?)", formatTime(s.now()), actor, action, detail)
	return err
}

func newID(prefix string, now time.Time) string {
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err == nil {
		return fmt.Sprintf("%s-%d-%x", prefix, now.UnixNano(), entropy)
	}
	return fmt.Sprintf("%s-%d", prefix, now.UnixNano())
}
