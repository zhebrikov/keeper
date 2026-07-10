// Package errors defines domain-level error values used across the application.
//
// These sentinel errors are mapped to gRPC status codes in the transport layer.
package errors

import "errors"

var (
	// ErrNotFound is returned when a requested entity does not exist.
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists is returned when creating a duplicate entity.
	ErrAlreadyExists = errors.New("already exists")
	// ErrInvalidCredentials is returned when login or password is incorrect.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrUnauthorized is returned when the caller is not authenticated.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrConflict is returned when an optimistic locking conflict occurs.
	ErrConflict = errors.New("version conflict")
	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = errors.New("invalid input")
)
