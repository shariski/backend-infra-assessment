package domain

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailTaken         = errors.New("email already registered")
	ErrUserNotFound       = errors.New("user not found")
	ErrTokenNotFound      = errors.New("refresh token not found")
	ErrTokenExpired       = errors.New("refresh token expired")
	ErrTokenRevoked       = errors.New("refresh token revoked")
	ErrAccountLocked      = errors.New("account temporarily locked due to too many failed login attempts")
	ErrForbidden          = errors.New("forbidden")
	ErrUnauthorized       = errors.New("unauthorized")
)
