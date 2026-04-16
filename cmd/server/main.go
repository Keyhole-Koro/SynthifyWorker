package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/synthify/backend/internal/app"
	"github.com/synthify/backend/internal/jobstatus"
	"github.com/synthify/backend/internal/middleware"
	"github.com/synthify/backend/internal/worker"
)

func main() {
	ctx := context.Background()
	port := envOrDefault("PORT", "8080")
	uploadURLBase := envOrDefault("GCS_UPLOAD_URL_BASE", "http://localhost:4443/synthify-uploads")

	store := app.InitStore(ctx, app.PublicUploadURLGenerator(uploadURLBase))
	notifier := jobstatus.NewNotifier(ctx, os.Getenv("FIREBASE_PROJECT_ID"))
	processor := worker.NewProcessorWithNotifier(store, store, notifier)

	mux := http.NewServeMux()
	mux.Handle("/internal/pipeline", worker.NewInternalHandler(processor, os.Getenv("INTERNAL_WORKER_TOKEN")))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Synthify Worker listening on %s", addr)
	if err := http.ListenAndServe(addr, middleware.Logger(mux)); err != nil {
		log.Fatal(err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
