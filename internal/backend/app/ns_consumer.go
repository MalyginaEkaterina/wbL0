package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-playground/validator/v10"
	"github.com/nats-io/stan.go"
	"log"
	"time"
	"wbL0/internal/backend"
	"wbL0/internal/backend/storage"
	"wbL0/internal/common"
)

type NSConsumer struct {
	storage storage.OrderStorage
	cache   *storage.Cache
	sc      stan.Conn
}

func NewNSConsumer(storage storage.OrderStorage, cache *storage.Cache, cfg backend.Config) (*NSConsumer, error) {
	sc, err := stan.Connect(cfg.StanClusterID, cfg.ClientID, stan.NatsURL(cfg.NatsURL))
	if err != nil {
		return nil, fmt.Errorf("error while connecting to NATS Streaming: %w", err)
	}

	return &NSConsumer{
		storage: storage,
		cache:   cache,
		sc:      sc,
	}, nil
}

func (c *NSConsumer) Run(ctx context.Context, cfg backend.Config) error {
	_, err := c.sc.Subscribe(cfg.ChannelName, c.msgHandle, stan.SetManualAckMode(), stan.DurableName(cfg.DurableName))
	if err != nil {
		return fmt.Errorf("subscribe error: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			er := c.sc.Close()
			if er != nil {
				return fmt.Errorf("close subscription error: %w", er)
			}
			log.Println("Stopped Nats Streaming  consumer")
			return nil
		}
	}
}

func (c *NSConsumer) msgHandle(m *stan.Msg) {
	//Отправляем Ack во всех случаях, кроме случая, когда не смогли сохранить в БД корректный заказ
	sendAck := true
	defer func() {
		if sendAck {
			err := m.Ack()
			if err != nil {
				log.Printf("Error while acknowledging a message: %v\n", err)
			}
		}
	}()
	var order common.Order
	err := json.Unmarshal(m.Data, &order)
	if err != nil {
		log.Printf("Error while unmarshal json: %v\n", err)
		return
	}
	//Проверяем обязательность некоторых json полей
	validate := validator.New()
	err = validate.Struct(order)
	if err != nil {
		log.Printf("Validate json error: %v\n", err)
		return
	}
	orderInfo, err := json.Marshal(order)
	if err != nil {
		log.Printf("Error while marshal order: %v\n", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err = c.storage.AddOrder(ctx, order.OrderUid, orderInfo)
	if errors.Is(err, storage.ErrAlreadyExists) {
		log.Printf("Order with id = %s already exists\n", order.OrderUid)
		return
	} else if err != nil {
		sendAck = false
		log.Printf("Error while adding order into db for %s: %v\n", order.OrderUid, err)
		return
	}
	err = c.cache.AddOrder(order.OrderUid, orderInfo)
	if err != nil {
		log.Printf("Error while adding order into cache for %s: %v\n", order.OrderUid, err)
		return
	}
}
