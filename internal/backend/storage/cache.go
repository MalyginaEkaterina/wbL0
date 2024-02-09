package storage

import (
	"context"
	"sync"
)

type Cache struct {
	orders map[string][]byte
	mutex  sync.RWMutex
}

func NewCache() *Cache {
	return &Cache{orders: make(map[string][]byte)}
}

func (c *Cache) FillFromStore(orderStorage OrderStorage) error {
	orders, err := orderStorage.GetOrders(context.Background())
	if err != nil {
		return err
	}
	for k, v := range orders {
		c.orders[k] = v
	}
	return nil
}

func (c *Cache) AddOrder(id string, order []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	_, ok := c.orders[id]
	if ok {
		return ErrAlreadyExists
	}
	c.orders[id] = order
	return nil
}

func (c *Cache) GetOrderByID(id string) ([]byte, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	order, ok := c.orders[id]
	if !ok {
		return nil, ErrNotFound
	}
	return order, nil
}
