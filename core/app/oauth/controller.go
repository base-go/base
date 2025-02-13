package oauth

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type OAuthController struct {
	Service *OAuthService
	Logger  *logrus.Logger
	Config  *OAuthConfig
}

func NewOAuthController(service *OAuthService, logger *logrus.Logger, config *OAuthConfig) *OAuthController {
	return &OAuthController{
		Service: service,
		Logger:  logger,
		Config:  config,
	}
}

func (c *OAuthController) Routes(router *gin.RouterGroup) {
	router.POST("/google/callback", c.GoogleCallback)
	router.POST("/facebook/callback", c.FacebookCallback)
	router.POST("/apple/callback", c.AppleCallback)
}

// GoogleCallback godoc
// @Summary Google OAuth callback
// @Description Handle the OAuth callback from Google
// @Security ApiKeyAuth
// @Tags Core/OAuth
// @Accept json
// @Produce json
// @Param idToken body string true "Google ID Token"
// @Success 200 {object} users.User
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /oauth/google/callback [post]
func (c *OAuthController) GoogleCallback(ctx *gin.Context) {
	var req struct {
		IDToken string `json:"idToken"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Logger.WithError(err).Error("Failed to bind JSON request")
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request payload"})
		return
	}

	user, err := c.Service.ProcessGoogleOAuth(req.IDToken)
	if err != nil {
		c.Logger.WithError(err).Error("Google OAuth authentication failed")
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, user)
}

// FacebookCallback godoc
// @Summary Facebook OAuth callback
// @Description Handle the OAuth callback from Facebook
// @Security ApiKeyAuth
// @Tags Core/OAuth
// @Accept json
// @Produce json
// @Param accessToken body string true "Facebook Access Token"
// @Success 200 {object} users.User
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /oauth/facebook/callback [post]
func (c *OAuthController) FacebookCallback(ctx *gin.Context) {
	var req struct {
		AccessToken string `json:"accessToken"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Logger.WithError(err).Error("Failed to bind JSON request")
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request payload"})
		return
	}

	user, err := c.Service.ProcessFacebookOAuth(req.AccessToken)
	if err != nil {
		c.Logger.WithError(err).Error("Facebook OAuth authentication failed")
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, user)
}

// AppleCallback godoc
// @Summary Apple OAuth callback
// @Description Handle the OAuth callback from Apple
// @Security ApiKeyAuth
// @Tags Core/OAuth
// @Accept json
// @Produce json
// @Param idToken body string true "Apple ID Token"
// @Success 200 {object} users.User
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /oauth/apple/callback [post]
func (c *OAuthController) AppleCallback(ctx *gin.Context) {
	var req struct {
		IDToken string `json:"idToken"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		c.Logger.WithError(err).Error("Failed to bind JSON request")
		ctx.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request payload"})
		return
	}

	user, err := c.Service.ProcessAppleOAuth(req.IDToken)
	if err != nil {
		c.Logger.WithError(err).Error("Apple OAuth authentication failed")
		ctx.JSON(http.StatusUnauthorized, ErrorResponse{Error: err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, user)
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}
