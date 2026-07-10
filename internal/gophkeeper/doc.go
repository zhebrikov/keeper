// Package gophkeeper documents the GophKeeper password manager module.
//
// # Overview
//
// GophKeeper is a client-server system for storing encrypted secrets
// (credentials, text, binary data, bank cards, OTP). Data is encrypted on
// the client with AES-GCM and a master password; the server stores only
// opaque encrypted blobs.
//
// # Layout
//
//	cmd/client, cmd/server — application entry points
//	internal/app — CLI and server bootstrap
//	internal/handler/grpc — gRPC transport layer
//	internal/service — business logic
//	internal/repository/postgres — PostgreSQL persistence
//	internal/client — gRPC client library and session storage
//	internal/tui — interactive terminal UI (Bubble Tea)
//	internal/crypto — bcrypt, PBKDF2, AES-GCM
//	internal/auth — JWT access tokens
//	internal/domain — models and domain errors
//	internal/config — environment-based configuration
//
// # Protocol
//
// The gRPC API is defined in api/proto/gophkeeper/v1/gophkeeper.proto.
// Protected methods require an authorization metadata header with a Bearer JWT.
package gophkeeper
