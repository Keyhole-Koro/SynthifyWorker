package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/Keyhole-Koro/SynthifyShared/app"
	"github.com/Keyhole-Koro/SynthifyShared/config"
	graphv1connect "github.com/Keyhole-Koro/SynthifyShared/gen/synthify/graph/v1/graphv1connect"
	"github.com/Keyhole-Koro/SynthifyShared/jobstatus"
	"github.com/Keyhole-Koro/SynthifyShared/middleware"
	"github.com/synthify/backend/worker/pkg/worker"
)

func main() {
	ctx := context.Background()
	cfg := config.LoadWorker()

	store := app.InitStore(ctx, app.PublicUploadURLGenerator(cfg.GCSUploadURLBase))
	notifier := jobstatus.NewNotifier(ctx, cfg.FirebaseProjectID)
	processor := worker.NewProcessorWithNotifier(store, store, notifier)

	mux := http.NewServeMux()
	mux.Handle(graphv1connect.NewWorkerServiceHandler(worker.NewConnectHandler(processor, cfg.InternalWorkerToken)))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("Synthify Worker listening on %s", addr)
	if err := http.ListenAndServe(addr, middleware.Logger(mux)); err != nil {
		log.Fatal(err)
	}
}
