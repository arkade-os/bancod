package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	introclient "github.com/ArkLabsHQ/introspector/pkg/client"
	arksdk "github.com/arkade-os/go-sdk"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/arkade-os/bancod/internal/config"
	"github.com/arkade-os/bancod/internal/core/application"
	sqlitedb "github.com/arkade-os/bancod/internal/infrastructure/db/sqlite"
	"github.com/arkade-os/bancod/internal/infrastructure/pricefeed"
	grpcservice "github.com/arkade-os/bancod/internal/interface/grpc"
	"github.com/arkade-os/bancod/pkg/banco"
	"github.com/arkade-os/bancod/pkg/solver"
)

// Version is injected at build time via -ldflags "-X main.Version=<tag>".
// Defaults to "dev" for local builds.
var Version = "dev"

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		logrus.WithError(err).Fatal("failed to load config")
	}

	log := logrus.New()
	log.SetLevel(logrus.Level(cfg.LogLevel))

	if err := os.MkdirAll(cfg.Datadir, 0750); err != nil {
		log.WithError(err).Fatal("failed to create datadir")
	}

	db, err := sqlitedb.OpenDB(cfg.Datadir)
	if err != nil {
		log.WithError(err).Fatal("failed to open database")
	}
	// nolint:errcheck
	defer db.Close()

	pairRepo := sqlitedb.NewPairRepository(db)
	tradeRepo := sqlitedb.NewTradeRepository(db)

	priceFeed := pricefeed.NewCoinGecko()

	introConn, err := grpc.NewClient(
		cfg.IntrospectorURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.WithError(err).Fatal("failed to connect to introspector")
	}
	// nolint:errcheck
	defer introConn.Close()
	introspector := introclient.NewGRPCClient(introConn)

	arkClient, err := arksdk.NewArkClient(cfg.Datadir)
	if err != nil {
		log.WithError(err).Fatal("failed to create ark client")
	}

	ctx := context.Background()
	if err := arkClient.Init(ctx, cfg.ArkURL, cfg.WalletSeed, cfg.WalletPassword); err != nil {
		log.WithError(err).Fatal("failed to init ark client")
	}
	if err := arkClient.Unlock(ctx, cfg.WalletPassword); err != nil {
		log.WithError(err).Fatal("failed to unlock ark client")
	}
	defer arkClient.Stop()

	tradeListener := application.NewTradeListener(tradeRepo, log)
	plugin := banco.NewPlugin(banco.Config{
		SolverClient:    arkClient,
		Introspector:    introspector,
		PairsRepository: pairRepo,
		PriceFeed:       priceFeed,
		Listener:        tradeListener,
		Log:             log,
	})
	s := solver.New(plugin).WithLogger(log)

	takerSvc := application.NewTakerService(s, pairRepo, tradeRepo, arkClient, arkClient.Indexer(), log)

	takerSvc.Start()

	srv := grpcservice.NewServer(takerSvc, cfg.GRPCPort, cfg.HTTPPort, log)
	if err := srv.Start(); err != nil {
		log.WithError(err).Fatal("failed to start server")
	}
	log.WithField("version", Version).Info("bancod started")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info("shutting down...")
	takerSvc.Stop()
	srv.Stop()
	log.Info("bancod stopped")
}
