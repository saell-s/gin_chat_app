package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "gin/docs"

	"gin/app/features/auth"
	"gin/app/features/call"
	"gin/app/features/chat"
	"gin/app/features/dashboard"
	"gin/app/features/reporting"
	"gin/app/features/user"
	"gin/app/shared/configs"
	"gin/app/shared/db"
	"gin/app/shared/kafka"
	"gin/app/shared/middleware"
	"gin/app/shared/realtime"
	"gin/app/shared/security"
	"gin/app/shared/utils"
	"gin/web"
)

// @title Gin Chat API
// @version 1.0
// @description REST API for a WhatsApp style messaging and calling app, built with Gin, PostgreSQL and JWT.
// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	cfg := configs.Load()
	if cfg.App.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	database, err := db.Open(ctx, cfg.Database)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(ctx, cfg.Database, security.HashPassword); err != nil {
		log.Fatalf("database migrate: %v", err)
	}

	pub := kafka.New(cfg.Kafka)
	defer pub.Close()

	tm := security.NewTokenManager(cfg.JWT)
	events := db.NewEventRepository(database)

	// Feature wiring: repository -> service -> handler.
	userRepo := user.NewRepository(database)
	userSvc := user.NewService(userRepo, events, pub)
	userHandler := user.NewHandler(userSvc)

	tokenRepo := auth.NewTokenRepository(database)
	authSvc := auth.NewService(userSvc, tokenRepo, tm, events, pub)
	authHandler := auth.NewHandler(authSvc)

	// Deleting an account or rotating its password invalidates its sessions.
	invalidate := func(ctx context.Context, userID int64) {
		if n, err := tokenRepo.RevokeAllForUser(ctx, userID); err == nil && n > 0 {
			log.Printf("revoked %d session(s) for user %d", n, userID)
		}
	}
	userSvc.OnPasswordChanged = invalidate
	userSvc.OnDeleted = invalidate

	dashSvc := dashboard.NewService(dashboard.NewStatsRepository(database), events)
	dashHandler := dashboard.NewHandler(dashSvc)

	reportSvc := reporting.NewService(reporting.NewRepository(database), events, pub)
	reportHandler := reporting.NewHandler(reportSvc)

	// Realtime: one hub for chat and calling.
	hub := realtime.New()
	chatSvc := chat.NewService(chat.NewRepository(database), userRepo, hub)
	chatHandler := chat.NewHandler(chatSvc, hub)
	callSvc := call.NewService(call.NewRepository(database), chatSvc, userRepo, hub, events, pub)
	callHandler := call.NewHandler(callSvc)

	hub.Router = routerFor(hub, chatSvc, callSvc)
	hub.OnConnect = func(c *realtime.Client) {
		c.Send(realtime.NewEvent("presence.snapshot", map[string]any{
			"online_ids": hub.OnlineIDs(),
		}))
	}

	authorize := middleware.Authorize(tm)

	r := gin.Default()
	r.SetTrustedProxies(nil)
	r.Use(middleware.RequestID(), middleware.CORS(cfg.CORS.AllowedOrigins))

	r.NoRoute(middleware.NotFound())

	// Frontend.
	web.RegisterRoutes(r)
	r.Static("/static", "./web/static")

	// Meta endpoints.
	r.GET("/health", health(database, hub))
	r.GET("/api/v1/test", func(c *gin.Context) {
		utils.OK(c, gin.H{"message": "API is working!"})
	})

	// Realtime endpoint: ?token=<access token> (browsers cannot set WS headers).
	r.GET("/ws", func(c *gin.Context) {
		raw := c.Query("token")
		if raw == "" {
			raw = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		}
		claims, err := tm.ParseAccess(raw)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{"message": "invalid or expired token", "status": http.StatusUnauthorized},
			})
			return
		}
		hub.Serve(c.Writer, c.Request, middleware.UserIDFromClaims(claims))
	})

	// Feature routes.
	api := r.Group("/api/v1")
	auth.RegisterRoutes(api, authHandler, authorize)
	user.RegisterRoutes(api, userHandler, authorize)
	dashboard.RegisterRoutes(api, dashHandler, authorize)
	reporting.RegisterRoutes(api, reportHandler, authorize)
	chat.RegisterRoutes(api, chatHandler, authorize)
	call.RegisterRoutes(api, callHandler, authorize)

	// Swagger UI.
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	log.Printf("%s listening on :%s (db=%s, kafka=%v, env=%s)",
		cfg.App.Name, cfg.App.Port, database.String(), pub.Enabled(), cfg.App.Env)
	if database.String() == "memory" && cfg.Database.Seed {
		log.Print("demo accounts: admin@example.com/Admin123! editor@example.com/Editor123! user@example.com/User12345!")
	}

	if err := r.Run(":" + cfg.App.Port); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}

// routerFor dispatches inbound websocket frames to the owning feature.
func routerFor(hub *realtime.Hub, chatSvc *chat.Service, callSvc *call.Service) func(userID int64, raw []byte) {
	return func(userID int64, raw []byte) {
		var frame struct {
			Type string         `json:"type"`
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(raw, &frame); err != nil || frame.Type == "" {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		var err error
		if strings.HasPrefix(frame.Type, "call.") {
			err = callSvc.HandleClientEvent(ctx, userID, frame.Type, frame.Data)
		} else {
			err = chatSvc.HandleClientEvent(ctx, userID, frame.Type, frame.Data)
		}
		if err != nil {
			hub.SendToUser(userID, realtime.NewEvent("error", map[string]any{
				"message": utils.MessageOf(err),
				"status":  utils.StatusOf(err),
				"event":   frame.Type,
			}))
		}
	}
}

func health(database *db.Database, hub *realtime.Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"db":     database.String(),
			"ws":     hub.ClientCount(),
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	}
}
