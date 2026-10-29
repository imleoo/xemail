package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"xemail/internal/config"
	"xemail/internal/handler"
	"xemail/internal/resendx"
	"xemail/internal/service"
	"xemail/internal/store"
	"xemail/internal/webhook"
	"xemail/migrations"
	"xemail/web"
)

const syncInterval = time.Minute

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o750); err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(migrations.FS); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc := service.New(st, cfg.AdminToken)
	if err := svc.SeedDomain(cfg.Domain); err != nil {
		log.Fatalf("XEMAIL_DOMAIN 不合法: %v", err)
	}
	if err := svc.Cleanup(); err != nil {
		log.Fatal(err)
	}
	inbox := service.NewInbox(ctx, st, resendx.New(cfg.ResendAPIKey))
	go inbox.RunLoop(syncInterval)

	static, err := web.Dist()
	if err != nil {
		log.Fatal(err)
	}

	deps := handler.Deps{Service: svc, Inbox: inbox, Static: static}
	if cfg.ResendWebhookSecret != "" {
		deps.Webhook = webhook.New(cfg.ResendWebhookSecret, st, inbox.Trigger)
	} else {
		log.Print("RESEND_WEBHOOK_SECRET 未设置，Webhook 未启用，仅靠每分钟轮询收信")
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler.NewRouter(deps),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("xemail listening on %s", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
