package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	firebase "firebase.google.com/go/v4"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("configuration: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	app, err := firebase.NewApp(ctx, &firebase.Config{
		ProjectID:   cfg.FirebaseProjectID,
		DatabaseURL: cfg.RTDBDatabaseURL,
	})
	if err != nil {
		log.Fatalf("initialize Firebase: %v", err)
	}

	authClient, err := app.Auth(ctx)
	if err != nil {
		log.Fatalf("initialize Firebase Authentication: %v", err)
	}
	rtdb, err := app.Database(ctx)
	if err != nil {
		log.Fatalf("initialize Realtime Database: %v", err)
	}
	firestoreClient, err := app.Firestore(ctx)
	if err != nil {
		log.Fatalf("initialize Firestore: %v", err)
	}
	defer firestoreClient.Close()

	handler := newAPI(cfg, authClient, rtdb, firestoreClient)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Firebase middleware listening on :%s", cfg.Port)
		serverErr <- server.ListenAndServe()
	}()

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server: %v", err)
		}
	case <-stopCtx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown: %v", err)
		}
	}
}
