package media

import (
	"base/core/emitter"
	"base/core/logger"
	"base/core/module"
	"base/core/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type MediaModule struct {
	module.DefaultModule
	DB            *gorm.DB
	Controller    *MediaController
	Service       *MediaService
	ActiveStorage *storage.ActiveStorage
	Emitter       *emitter.Emitter
	Logger        logger.Logger
}

func NewMediaModule(
	db *gorm.DB,
	router *gin.RouterGroup,
	activeStorage *storage.ActiveStorage,
	emitter *emitter.Emitter,
	logger logger.Logger,
) module.Module {
	service := NewMediaService(db, emitter, activeStorage, logger)
	controller := NewMediaController(service, activeStorage, logger)

	mediaModule := &MediaModule{
		DB:            db,
		Controller:    controller,
		Service:       service,
		ActiveStorage: activeStorage,
		Emitter:       emitter,
		Logger:        logger,
	}

	return mediaModule
}

func (m *MediaModule) Routes(router *gin.RouterGroup) {
	m.Logger.Info("Registering media module routes")
	mediaGroup := router.Group("/media")
	m.Controller.Routes(mediaGroup)
	m.Logger.Info("Media module routes registered")
}

func (m *MediaModule) Migrate() error {
	return m.DB.AutoMigrate(&Media{})
}

func (m *MediaModule) GetModels() []interface{} {
	return []interface{}{&Media{}}
}
