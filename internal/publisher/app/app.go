package app

import (
	"encoding/json"
	"fmt"
	"github.com/caarlos0/env/v6"
	"github.com/google/uuid"
	"github.com/nats-io/stan.go"
	"log"
	"os"
	"wbL0/internal/common"
	"wbL0/internal/publisher"
)

func Start() {
	var cfg publisher.Config
	err := env.Parse(&cfg)
	if err != nil {
		log.Fatal("Error while parsing env: ", err)
	}
	id := uuid.NewString()
	msg, err := GenerateMsg("msg.json", id)
	//msg, err := GenerateMsg("wrongMsg.json", id)
	if err != nil {
		log.Fatal("Error while generate msg: ", err)
	}
	err = Publish(cfg, msg)
	if err != nil {
		log.Fatal("Error while publish: ", err)
	}
	log.Printf("Order with id = %s was published", id)
}

func Publish(cfg publisher.Config, msg []byte) error {
	sc, err := stan.Connect(cfg.StanClusterID, cfg.ClientID, stan.NatsURL(cfg.NatsURL))
	if err != nil {
		log.Println(err)
		return fmt.Errorf("error while connecting to NATS Streaming: %w", err)
	}
	err = sc.Publish(cfg.ChannelName, msg)
	if err != nil {
		return fmt.Errorf("error while publishing second msg: %w", err)
	}
	return nil
}

func GenerateMsg(msgName string, id string) ([]byte, error) {
	msg, err := os.ReadFile(msgName)
	if err != nil {
		return nil, fmt.Errorf("error while reading file with msg: %w", err)
	}
	var order common.Order
	err = json.Unmarshal(msg, &order)
	if err != nil {
		return nil, fmt.Errorf("error while parsing msg from file: %w", err)
	}
	order.OrderUid = id
	order.Payment.Transaction = id
	orderInfo, err := json.Marshal(order)
	if err != nil {
		return nil, fmt.Errorf("error while marshal order: %w", err)
	}
	return orderInfo, nil
}
