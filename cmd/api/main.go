package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"taskforge/internal/api"
	"taskforge/internal/auth"
	"taskforge/internal/database"
	"taskforge/internal/jobs"
	"taskforge/internal/repository"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	startupCtx, startupCancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer startupCancel()

	// Initialize PostgreSQL.
	dbConfig, err := database.ConfigFromEnv()
	if err != nil {
		return errors.New("database configuration failed")
	}

	pool, err := database.Open(startupCtx, dbConfig)
	if err != nil {
		return errors.New("database initialization failed")
	}
	defer pool.Close()

	// Initialize job repository and service.
	jobRepo := repository.NewPostgresJobRepository(pool)

	jobService := jobs.NewService(
		jobRepositoryAdapter{
			PostgresJobRepository: jobRepo,
		},
	)

	// Initialize native JWT authentication.
	jwtSecret := os.Getenv("JWT_SECRET")
	jwtIssuer := os.Getenv("JWT_ISSUER")
	jwtAudience := os.Getenv("JWT_AUDIENCE")

	authenticator, err := auth.NewJWTAuthenticator(
		jwtSecret,
		jwtIssuer,
		jwtAudience,
	)
	if err != nil {
		return errors.New("JWT authentication initialization failed")
	}

	// Initialize user repository.
	userRepo := repository.NewUserRepository(pool)

	refreshRepo := repository.NewRefreshTokenRepository(pool)

	// Initialize authentication service.
	authService := auth.NewAuthService(
		userRepo,
		authenticator,
		refreshRepo,
	)

	// Register API routes.
	apiHandler := api.NewRouter(
		jobService,
		authService,
		authenticator,
	)

	handler := api.RecoverMiddleware(
		api.RequestLogger(apiHandler),
	)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Handle operating-system shutdown signals.
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	serverErr := make(chan error, 1)

	go func() {
		slog.Info("API server starting", "addr", server.Addr)

		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}

		serverErr <- nil
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			return errors.New("HTTP server failed")
		}
		return nil

	case <-ctx.Done():
		slog.Info("shutting down API server")
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return errors.New("graceful HTTP shutdown failed")
	}

	return nil
}

// jobRepositoryAdapter adapts the PostgreSQL repository
// to the jobs service.
type jobRepositoryAdapter struct {
	*repository.PostgresJobRepository
}

// GetByID preserves tenant isolation.
func (r jobRepositoryAdapter) GetByID(
	ctx context.Context,
	id string,
	tenantID string,
) (jobs.Job, error) {
	return r.PostgresJobRepository.GetByID(
		ctx,
		id,
		tenantID,
	)
}
