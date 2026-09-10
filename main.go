package main

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"nfl-quiniela-2026/config"
	"nfl-quiniela-2026/db"
	"nfl-quiniela-2026/handlers"
	"nfl-quiniela-2026/services/auth"
	"nfl-quiniela-2026/services/espn"
	"nfl-quiniela-2026/services/events"
	"nfl-quiniela-2026/services/notifications"
	"nfl-quiniela-2026/services/scoring"
)

//go:embed templates/* templates/layouts/* templates/pages/* templates/partials/*
var templatesFS embed.FS

//go:embed static/* static/css/* static/js/*
var staticFS embed.FS

func main() {
	log.Println("==========================================")
	log.Println("🏈 NFL Quiniela 2026 - Starting Server...")
	log.Println("==========================================")

	// 1. Configuration
	cfg := config.LoadConfig()

	// 2. Database Initialization
	var dsn string
	if cfg.DBType == "postgres" && cfg.DatabaseURL != "" {
		dsn = cfg.DatabaseURL
	} else {
		dsn = cfg.DBPath
	}

	database, err := db.InitDB(cfg.DBType, dsn)
	if err != nil {
		log.Fatalf("Fatal: Database initialization failed: %v", err)
	}
	defer database.Close()

	repo := db.NewRepository(database)

	// 3. Database Seeds
	if err := db.SeedDatabase(repo, cfg.AdminUsername, cfg.AdminEmail, cfg.AdminPassword, cfg.CurrentSeasonYear); err != nil {
		log.Printf("Warning: Seed database error: %v", err)
	}

	// 4. Core Services & Real-time Event Broker
	authService := auth.NewAuthService(repo, cfg.SessionSecret)
	calculator := scoring.NewCalculator(repo)
	broker := events.NewBroker()
	espnClient := espn.NewClient()
	syncer := espn.NewSyncer(espnClient, repo, calculator, broker, cfg.CurrentSeasonYear)

	// Notifications & Reminder Worker
	emailSender := notifications.NewEmailSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom, cfg.AppBaseURL)
	reminderWorker := notifications.NewReminderWorker(repo, emailSender, cfg.CurrentSeasonYear)

	// 5. Background Tasks (Sync, Heartbeat, Reminders)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	broker.StartHeartbeat(ctx, 25*time.Second)

	if cfg.EnableBackgroundSync {
		syncer.StartBackgroundSync(ctx, time.Duration(cfg.ESPNSyncIntervalMins)*time.Minute)
	}

	if cfg.EnableReminders {
		reminderWorker.Start(ctx, 15*time.Minute)
	}

	// Trigger initial sync for Week 1
	go func() {
		log.Println("[Syncer] Performing initial startup sync for Week 1...")
		if count, err := syncer.SyncWeek(1); err != nil {
			log.Printf("[Syncer] Initial sync note: %v", err)
		} else {
			log.Printf("[Syncer] Initial sync completed: %d games ready for Week 1.", count)
		}
	}()

	// 6. Template Renderer
	subTemplatesFS, err := fs.Sub(templatesFS, "templates")
	if err != nil {
		log.Fatalf("Fatal: Failed to load templates filesystem: %v", err)
	}
	renderer := handlers.NewRenderer(subTemplatesFS)

	// 7. HTTP Handlers
	authHandler := handlers.NewAuthHandler(authService, repo, emailSender, renderer)
	profileHandler := handlers.NewProfileHandler(repo, authService, renderer)
	picksHandler := handlers.NewPicksHandler(repo, renderer, syncer, cfg.CurrentSeasonYear)
	leaderboardHandler := handlers.NewLeaderboardHandler(repo, renderer, cfg.CurrentSeasonYear)
	rulesHandler := handlers.NewRulesHandler(repo, renderer)
	adminHandler := handlers.NewAdminHandler(repo, renderer, syncer, calculator, broker, reminderWorker, cfg.CurrentSeasonYear)
	eventsHandler := handlers.NewEventsHandler(broker)

	// 8. Router Setup
	r := chi.NewRouter()

	// Global Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(authService.AuthMiddleware)

	// Static Files (/static/*)
	subStaticFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("Fatal: Failed to load static filesystem: %v", err)
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(subStaticFS))))

	// Public Routes
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		dbStatus := "connected"
		status := "ok"
		statusCode := http.StatusOK
		if err := database.Ping(); err != nil {
			dbStatus = "error: " + err.Error()
			status = "degraded"
			statusCode = http.StatusServiceUnavailable
		} else if database.DriverName == "sqlite" {
			var checkResult string
			if err := database.QueryRow("PRAGMA quick_check").Scan(&checkResult); err != nil || checkResult != "ok" {
				dbStatus = "integrity check note: " + checkResult
			}
		}
		isWritable := database.IsStorageWritable()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":           status,
			"database":         dbStatus,
			"driver":           database.DriverName,
			"db_path":          database.DBPath,
			"storage_writable": isWritable,
			"season":           cfg.CurrentSeasonYear,
			"timestamp":        time.Now().UTC().Format(time.RFC3339),
		})
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/picks", http.StatusSeeOther)
	})
	r.Get("/login", authHandler.ShowLogin)
	r.Post("/login", authHandler.HandleLogin)
	r.Get("/register", authHandler.ShowRegister)
	r.Post("/register", authHandler.HandleRegister)
	r.Post("/logout", authHandler.HandleLogout)

	// Email Verification & Password Reset
	r.Get("/verify-email", authHandler.HandleVerifyEmail)
	r.Post("/verify-email/resend", authHandler.HandleResendVerification)
	r.Get("/forgot-password", authHandler.ShowForgotPassword)
	r.Post("/forgot-password", authHandler.HandleForgotPassword)
	r.Get("/reset-password", authHandler.ShowResetPassword)
	r.Post("/reset-password", authHandler.HandleResetPassword)

	r.Get("/rules", rulesHandler.ShowRules)
	r.Get("/leaderboard", leaderboardHandler.ShowLeaderboard)
	r.Get("/leaderboard/table", leaderboardHandler.LeaderboardTable)
	r.Get("/events/live", eventsHandler.StreamLiveEvents)

	// Authenticated Player Routes
	r.Group(func(player chi.Router) {
		player.Use(authService.RequireAuth)

		player.Get("/picks", picksHandler.ShowPicks)
		player.Post("/picks/save", picksHandler.SavePick)
		player.Post("/picks/save-score", picksHandler.SaveScore)
		player.Post("/picks/save-all", picksHandler.SaveAll)
		player.Get("/picks/community/{gameId}", picksHandler.CommunityPicks)

		// Profile & Preferences
		player.Get("/profile", profileHandler.ShowProfile)
		player.Post("/profile/preferences", profileHandler.HandleUpdatePreferences)
		player.Post("/profile/password", profileHandler.HandleChangePassword)
	})

	// Admin Protected Routes
	r.Group(func(admin chi.Router) {
		admin.Use(authService.RequireAuth)
		admin.Use(authService.RequireAdmin)

		admin.Get("/admin", adminHandler.ShowAdmin)
		admin.Post("/admin/settings/save", adminHandler.SaveSettings)
		admin.Post("/admin/sync-espn", adminHandler.SyncESPN)
		admin.Post("/admin/games/save-score", adminHandler.SaveGameScore)
		admin.Post("/admin/games/toggle-lock", adminHandler.ToggleGameLock)
		admin.Post("/admin/games/toggle-tiebreaker", adminHandler.ToggleTiebreaker)
		admin.Post("/admin/reminders/send", adminHandler.SendReminders)
	})

	// 9. HTTP Server & Graceful Shutdown
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("🚀 Server listening on http://localhost:%s", cfg.Port)
		log.Printf("🔑 Default admin credentials -> User: %s | Pass: %s", cfg.AdminUsername, cfg.AdminPassword)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server ListenAndServe error: %v", err)
		}
	}()

	// Graceful shutdown listener
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)
	<-stopChan

	log.Println("Shutting down server gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	// For SQLite, perform WAL checkpoint to flush all journals into the main database file
	if database.DriverName == "sqlite" {
		log.Println("[DB] Checkpointing SQLite WAL log to main database before termination...")
		if err := database.CheckpointWAL(); err != nil {
			log.Printf("[DB] Note on WAL checkpoint: %v", err)
		} else {
			log.Println("[DB] SQLite WAL checkpoint completed successfully.")
		}
	}

	log.Println("Server stopped.")
}
