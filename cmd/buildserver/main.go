// Command buildserver is the private update/download server this app's
// internal/updater client talks to — see internal/buildserver's package
// doc. Deployed on Alhena; configured entirely via environment variables
// so the two real secrets (the download token every client embeds, and
// the higher-privilege publish token) never need to live in a config file
// on disk or in any tracked source.
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

	"liforra-tool/internal/buildserver"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	dataDir := env("BUILDSERVER_DATA_DIR", "./data")
	listenAddr := env("BUILDSERVER_LISTEN_ADDR", ":8080")
	downloadToken := os.Getenv("BUILDSERVER_DOWNLOAD_TOKEN")
	publishToken := os.Getenv("BUILDSERVER_PUBLISH_TOKEN")

	if downloadToken == "" {
		log.Fatal("BUILDSERVER_DOWNLOAD_TOKEN is required (must match every compiled client's embedded token)")
	}
	if publishToken == "" {
		log.Fatal("BUILDSERVER_PUBLISH_TOKEN is required (a separate, higher-privilege secret — never shipped in any client)")
	}

	store, err := buildserver.NewStore(dataDir)
	if err != nil {
		log.Fatalf("opening data dir %q: %v", dataDir, err)
	}

	srv := &buildserver.Server{Store: store, DownloadToken: downloadToken, PublishToken: publishToken}
	httpServer := &http.Server{
		Addr:              listenAddr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("buildserver listening on %s (data: %s)", listenAddr, dataDir)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log.Println("shutting down...")
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Println("shutdown error:", err)
	}
}
