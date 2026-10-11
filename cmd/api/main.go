package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"task-manager/internal/task"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *responseRecorder) WriteHeader(code int) {
	if r.statusCode != 0 {
		return
	}
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(body []byte) (int, error) {
	if r.statusCode == 0 {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(body)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &responseRecorder{ResponseWriter: w}

		next.ServeHTTP(recorder, r)

		status := recorder.statusCode
		if status == 0 {
			status = http.StatusOK
		}

		log.Printf(
			"method=%s path=%s status=%d duration=%s",
			r.Method,
			r.URL.Path,
			status,
			time.Since(start),
		)
	})
}
func loadDatabaseURL() (string, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return "", errors.New("DATABASE_URL не задана")
	}
	return dsn, nil
}
func openDatabase(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	err = pool.Ping(ctx)

	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}
func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()
	dsn, err := loadDatabaseURL()
	if err != nil {
		fmt.Println(err)
		return
	}
	connectCtx, cancelConnect := context.WithTimeout(ctx, 5*time.Second)
	pool, err := openDatabase(connectCtx, dsn)
	cancelConnect()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer pool.Close()
	fmt.Println("Подключение к PostgreSQL установлено")
	repository := task.NewPostgresRepository(pool)
	service := task.NewService(repository)
	handler := task.NewHTTPHandler(service)
	mux := http.NewServeMux()
	mux.Handle("/", handler.Routes())

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	server := &http.Server{
		Addr:              ":8080",
		Handler:           loggingMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)

	go func() {
		serverErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Println("Получен сигнал остановки")

	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Ошибка HTTP-сервера: %v", err)
		}
		return
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Ошибка завершения HTTP-сервера: %v", err)
		if closeErr := server.Close(); closeErr != nil {
			log.Printf("Ошибка закрытия HTTP-сервера: %v", closeErr)
		}
	}
	log.Println("HTTP-сервер остановлен")
}
