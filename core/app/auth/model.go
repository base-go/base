package auth

import (
	"base/core/app/users"
	"base/core/storage"
	"time"
)

type AuthUser struct {
	users.User       `gorm:"embedded"`
	LastLogin        *time.Time `gorm:"column:last_login"`
	ResetToken       string     `gorm:"column:reset_token"`
	ResetTokenExpiry *time.Time `gorm:"column:reset_token_expiry"`
}

func (AuthUser) TableName() string {
	return "users"
}

type LoginEvent struct {
	User         *AuthUser
	LoginAllowed *bool
}
type RegisterRequest struct {
	Name     string `json:"name" gorm:"column:name"`
	Username string `json:"username" gorm:"column:username"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetPasswordRequest struct {
	Email       string `json:"email" binding:"required,email"`
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

type AuthResponse struct {
	AccessToken string              `json:"accessToken"`
	Exp         int64               `json:"exp"`
	Username    string              `json:"username"`
	ID          uint                `json:"id"`
	Avatar      *storage.Attachment `json:"avatar"`
	Email       string              `json:"email"`
	Name        string              `json:"name"`
	LastLogin   string              `json:"last_login"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Message string `json:"message"`
}

// VerifyOTPRequest represents the payload to verify an OTP for login
type VerifyOTPRequest struct {
	Email string `json:"email" binding:"required,email"`
	OTP   string `json:"otp" binding:"required"`
}

// SendOTPRequest represents the payload to request sending an OTP
type SendOTPRequest struct {
	Email string `json:"email" binding:"required,email"`
}
