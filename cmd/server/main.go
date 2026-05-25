// Command server is the GophKeeper gRPC server.
package main

import (
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/arsykor/gophkeeper/internal/server/auth"
	servergrpc "github.com/arsykor/gophkeeper/internal/server/grpc"
	"github.com/arsykor/gophkeeper/internal/server/service"
	"github.com/arsykor/gophkeeper/internal/server/storage/minio"
	"github.com/arsykor/gophkeeper/internal/server/storage/postgres"
	"github.com/arsykor/gophkeeper/migrations"
	pb "github.com/arsykor/gophkeeper/proto/gophkeeper/v1"

	serverconfig "github.com/arsykor/gophkeeper/internal/server/config"
)

// Version and BuildDate are set by -ldflags at build time.
var (
	Version   = "dev"
	BuildDate = "unknown"
)

func main() {
	log, _ := zap.NewProduction()
	defer log.Sync() //nolint:errcheck
	sugar := log.Sugar()

	sugar.Infow("starting gophkeeper server", "version", Version, "built", BuildDate)

	cfg, err := serverconfig.Load()
	if err != nil {
		sugar.Fatalw("failed to load config", "error", err)
	}

	// Postgres
	db, err := postgres.New(cfg.DatabaseDSN, migrations.FS)
	if err != nil {
		sugar.Fatalw("failed to connect to postgres", "error", err)
	}
	defer db.Close()

	// MinIO
	minioClient, err := minio.New(cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioBucket, cfg.MinioUseSSL)
	if err != nil {
		sugar.Fatalw("failed to connect to minio", "error", err)
	}

	// Auth
	authMgr := auth.New(cfg.JWTSecret)
	enc, err := auth.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		sugar.Fatalw("invalid encryption key", "error", err)
	}

	// Service
	svc := service.New(db, db, minioClient, authMgr, enc)

	// gRPC
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(servergrpc.AuthInterceptor(authMgr)),
		grpc.StreamInterceptor(servergrpc.StreamAuthInterceptor(authMgr)),
	)
	pb.RegisterAuthServiceServer(grpcServer, servergrpc.NewAuthHandler(svc))
	pb.RegisterSecretServiceServer(grpcServer, servergrpc.NewSecretHandler(svc))

	lis, err := net.Listen("tcp", cfg.GRPCAddress)
	if err != nil {
		sugar.Fatalw("failed to listen", "address", cfg.GRPCAddress, "error", err)
	}

	go func() {
		sugar.Infow("gRPC server listening", "address", cfg.GRPCAddress)
		if err := grpcServer.Serve(lis); err != nil {
			sugar.Errorw("gRPC server error", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	sugar.Info("shutting down gracefully")
	grpcServer.GracefulStop()
}
