package core

import (
	"base/app"
	"base/core/config"
	"base/core/database"
	"base/core/email"
	"base/core/emitter"
	"base/core/event"
	"base/core/storage"

	"base/core/middleware"
	"base/core/module"
	"base/core/websocket"
	_ "base/docs" // Import for Swagger docs
	"context"
	"fmt"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
)

type Application struct {
	Config       *config.Config
	DB           *database.Database
	Router       *gin.Engine
	WSHub        *websocket.Hub
	Modules      map[string]module.Module
	Logger       *zap.Logger
	EventService *event.EventService
	Emitter      *emitter.Emitter
}

var Emitter = emitter.New() // This ensures Emitter is created once
func StartApplication() (*Application, error) {
	ctx := context.Background()

	// Initialize config
	cfg := config.NewConfig()

	// Initialize Zap logger first
	logger := InitializeLogger()
	defer logger.Sync()

	logger.Info("Starting application initialization",
		zap.String("version", cfg.Version),
		zap.String("environment", cfg.Env))

	// Initialize the database
	db, err := database.InitDB(cfg)
	if err != nil {
		logger.Error("Failed to initialize database", zap.Error(err))
		return nil, fmt.Errorf("database initialization failed: %w", err)
	}
	logger.Info("Database initialized successfully")

	// Initialize and verify event service
	logger.Info("Initializing event service")
	if err := db.DB.AutoMigrate(&event.Event{}); err != nil {
		logger.Error("Failed to migrate events table", zap.Error(err))
		return nil, fmt.Errorf("events table migration failed: %w", err)
	}

	if !db.DB.Migrator().HasTable(&event.Event{}) {
		logger.Error("Events table does not exist after migration")
		return nil, fmt.Errorf("events table creation failed")
	}

	eventService := event.NewEventService(db.DB, logger)

	// Track initialization start
	_, err = eventService.Track(ctx, event.EventOptions{
		Type:        "system",
		Category:    "startup",
		Actor:       "system",
		Action:      "start",
		Status:      "started",
		Description: "Application initialization started",
		Metadata: map[string]interface{}{
			"version": cfg.Version,
			"env":     cfg.Env,
		},
	})
	if err != nil {
		logger.Error("Failed to track initial event", zap.Error(err))
		return nil, fmt.Errorf("event tracking failed: %w", err)
	}
	logger.Info("Event service initialized successfully")

	// Initialize email sender
	emailSender, err := email.NewSender(cfg)
	if err != nil {
		logger.Error("Failed to initialize email sender", zap.Error(err))
		return nil, fmt.Errorf("email sender initialization failed: %w", err)
	}
	logger.Info("Email sender initialized successfully")

	// Initialize storage service
	logger.Info("Initializing storage service")
	storageConfig := storage.Config{
		Provider:  cfg.StorageProvider,
		Path:      cfg.StoragePath,
		BaseURL:   cfg.BaseURL,
		APIKey:    cfg.StorageAPIKey,
		APISecret: cfg.StorageAPISecret,
		Endpoint:  cfg.StorageEndpoint,
		Bucket:    cfg.StorageBucket,
		CDN:       cfg.CDN,
	}

	activeStorage, err := storage.NewActiveStorage(db.DB, storageConfig)
	if err != nil {
		logger.Error("Failed to initialize storage service", zap.Error(err))
		return nil, fmt.Errorf("storage service initialization failed: %w", err)
	}

	// Register attachments configuration
	activeStorage.RegisterAttachment("users", storage.AttachmentConfig{
		Field:             "avatar",
		Path:              "users",
		AllowedExtensions: []string{".jpg", ".jpeg", ".png"},
		MaxFileSize:       2 << 20, // 2MB
		Multiple:          false,
	})

	activeStorage.RegisterAttachment("users", storage.AttachmentConfig{
		Field:             "documents",
		Path:              "users",
		AllowedExtensions: []string{".pdf", ".doc", ".docx"},
		MaxFileSize:       10 << 20, // 10MB
		Multiple:          true,
	})

	logger.Info("Storage service initialized successfully",
		zap.String("provider", cfg.StorageProvider),
		zap.String("path", cfg.StoragePath))

	// Set up Gin
	router := gin.New()
	router.Use(gin.Recovery())

	// Set up middleware
	router.Use(middleware.EventTrackingMiddleware(eventService))
	router.Use(middleware.ZapLogger(logger))

	// Set up static file serving
	router.Static("/static", "./static")
	router.Static("/admin", "./admin")
	router.Static("/storage", "./storage")

	// Set up CORS
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = cfg.CORSAllowedOrigins
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Api-Key"}
	router.Use(cors.New(corsConfig))

	// Set up Swagger
	router.GET("/swagger/*any",
		ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.PersistAuthorization(true)))
	logger.Info("Swagger documentation enabled")

	// Create API router group
	apiGroup := router.Group("/api")
	apiGroup.Use(middleware.APIKeyMiddleware())

	// Initialize core modules with all dependencies
	logger.Info("Initializing core modules")
	moduleInit := ModuleInitializer{
		DB:           db.DB,
		Router:       apiGroup,
		EmailSender:  emailSender,
		Logger:       logger,
		EventService: eventService,
		Emitter:      Emitter,
		Storage:      activeStorage,
	}

	modules := InitializeCoreModules(moduleInit)
	logger.Info("Core modules initialized", zap.Int("count", len(modules)))

	// Initialize application modules
	logger.Info("Initializing application modules")
	appInitializer := &app.AppModuleInitializer{
		Router:  apiGroup,
		Logger:  logger,
		Emitter: Emitter,
	}
	appInitializer.InitializeModules(db.DB)

	// Add health check route
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"version": cfg.Version,
		})
	})

	// Add ping route
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
			"version": cfg.Version,
			"swagger": "/swagger/index.html",
		})
	})

	// Initialize WebSocket module
	logger.Info("Initializing WebSocket module")
	wsHub := websocket.InitWebSocketModule(apiGroup)

	// Create application instance
	application := &Application{
		Config:       cfg,
		DB:           db,
		Router:       router,
		WSHub:        wsHub,
		Modules:      modules,
		Logger:       logger,
		EventService: eventService,
		Emitter:      Emitter,
	}

	// Track successful startup
	_, err = eventService.Track(ctx, event.EventOptions{
		Type:        "system",
		Category:    "startup",
		Actor:       "system",
		Action:      "start",
		Status:      "completed",
		Description: "Application started successfully",
		Metadata: map[string]interface{}{
			"version":      cfg.Version,
			"environment":  cfg.Env,
			"module_count": len(modules),
		},
	})
	if err != nil {
		logger.Error("Failed to track startup completion", zap.Error(err))
		// Don't fail the startup for this error
	}

	logger.Info("Application started successfully",
		zap.String("version", cfg.Version),
		zap.String("environment", cfg.Env),
		zap.Int("module_count", len(modules)))

	fmt.Println(cfg)
	return application, nil
}
