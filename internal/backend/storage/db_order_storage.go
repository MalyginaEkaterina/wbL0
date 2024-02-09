package storage

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/lib/pq"
)

type DBOrderStorage struct {
	db          *sql.DB
	insertOrder *sql.Stmt
	getOrders   *sql.Stmt
}

var _ OrderStorage = (*DBOrderStorage)(nil)

func NewDBOrderStorage(db *sql.DB) (*DBOrderStorage, error) {
	stmtInsertOrder, err := db.Prepare("INSERT INTO orders (id, order_info) VALUES ($1, $2)")
	if err != nil {
		return nil, err
	}
	stmtGetOrders, err := db.Prepare("SELECT * from orders")
	if err != nil {
		return nil, err
	}

	return &DBOrderStorage{
		db:          db,
		insertOrder: stmtInsertOrder,
		getOrders:   stmtGetOrders,
	}, nil
}

func (d *DBOrderStorage) AddOrder(ctx context.Context, id string, order []byte) error {
	_, err := d.insertOrder.ExecContext(ctx, id, order)
	if err != nil {
		if IsDuplicateKeyError(err) {
			return ErrAlreadyExists
		}
		return fmt.Errorf("insert order error: %w", err)
	}
	return nil
}

func IsDuplicateKeyError(err error) bool {
	pgErr, ok := err.(*pq.Error)
	return ok && pgErr.Code == "23505"
}

func (d *DBOrderStorage) GetOrders(ctx context.Context) (map[string][]byte, error) {
	rows, err := d.getOrders.QueryContext(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	orders := make(map[string][]byte)
	for rows.Next() {
		var id string
		var order []byte
		err = rows.Scan(&id, &order)
		if err != nil {
			return nil, err
		}
		orders[id] = order
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	if len(orders) == 0 {
		return nil, ErrNotFound
	}

	return orders, nil
}

func (d *DBOrderStorage) Close() {
	d.insertOrder.Close()
	d.getOrders.Close()
}
