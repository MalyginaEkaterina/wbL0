package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/caarlos0/env/v6"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/sync/errgroup"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"wbL0/internal/backend"
	"wbL0/internal/backend/storage"
)

const (
	idKey = "id"
)

func Start() {
	//Заполняем конфиг из env переменных
	var cfg backend.Config
	err := env.Parse(&cfg)
	if err != nil {
		log.Fatal("Error while parsing env: ", err)
	}

	//Подключаемся к БД, накатываем миграции
	log.Printf("DATABASE_URI %q\n", cfg.DatabaseURI)
	db, orderStorage, err := initStore(cfg)
	if err != nil {
		log.Fatal("Init store error: ", err)
	}
	defer func() {
		if db != nil {
			db.Close()
		}
	}()
	defer orderStorage.Close()
	//Инициализируем кеш и заполняем его и БД
	cache := storage.NewCache()
	err = cache.FillFromStore(orderStorage)
	if errors.Is(err, storage.ErrNotFound) {
		log.Println("Cache is empty")
	} else if err != nil {
		log.Fatal("Fill cache error: ", err)
	}

	nsConsumer, err := NewNSConsumer(orderStorage, cache, cfg)
	if err != nil {
		log.Fatal("Create nsConsumer error: ", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", mainHandler)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		getOrderHandler(w, r, cache)
	})
	server := &http.Server{Addr: cfg.Address, Handler: mux}

	//Обрабатываем сигналы для корректного завершения
	signalCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer cancel()

	//В данной задаче это неважно, поэтому останавливаем nats consumer и http server в любом порядке
	grp, ctx := errgroup.WithContext(signalCtx)
	grp.Go(func() error {
		log.Printf("Started server on %s\n", cfg.Address)
		go func() {
			<-ctx.Done()
			if er := server.Shutdown(context.Background()); er != nil {
				log.Printf("Failed to shutdown server: %v\n", er)
			}
		}()
		if er := server.ListenAndServe(); er != http.ErrServerClosed {
			return fmt.Errorf("HTTP server ListenAndServe: %w", er)
		}
		log.Printf("Stopped server on %s\n", cfg.Address)
		return nil
	})
	grp.Go(func() error {
		log.Printf("Started NSConsumer\n")
		if er := nsConsumer.Run(ctx, cfg); er != nil {
			return fmt.Errorf("NSConsumer Run: %w", er)
		}
		return nil
	})
	err = grp.Wait()
	if err != nil {
		log.Printf("Got error: %v\n", err)
	}
}

func mainHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	http.ServeFile(w, r, "index.html")
}

// getOrderHandler обрабатывает запрос информации по заказу по id, возвращает код 400 в случае, если заказа с таким id не существует
func getOrderHandler(w http.ResponseWriter, r *http.Request, cache *storage.Cache) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET requests are allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get(idKey)
	if id == "" {
		http.Error(w, "Order ID is required", http.StatusBadRequest)
		return
	}
	order, err := cache.GetOrderByID(id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "Not found", http.StatusBadRequest)
		} else {
			log.Println("Error while getting order", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(order)
}

func initStore(cfg backend.Config) (*sql.DB, storage.OrderStorage, error) {
	db, err := sql.Open("postgres", cfg.DatabaseURI)
	if err != nil {
		return nil, nil, fmt.Errorf("database connection error: %w", err)
	}
	err = storage.DoMigrations(db)
	if err != nil {
		return nil, nil, fmt.Errorf("running migrations error: %w", err)
	}
	log.Printf("Using database storage %s\n", cfg.DatabaseURI)
	dbOrderStorage, err := storage.NewDBOrderStorage(db)
	if err != nil {
		return nil, nil, fmt.Errorf("create order storage error: %w", err)
	}
	return db, dbOrderStorage, nil
}
