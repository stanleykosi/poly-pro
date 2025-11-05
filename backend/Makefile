# Makefile for Poly-Pro Analytics Backend Service
# ------------------------------------------------
# This file provides convenience commands for common development tasks,
# such as database migrations and running the server.
#
# Usage:
#   make migrate-create NAME=my_new_migration  - Creates new up/down migration files.
#   make migrate-up                           - Applies all pending 'up' migrations.
#
# Notes:
# - It is expected that the `golang-migrate/migrate` CLI is installed.
# - The `DATABASE_URL` environment variable must be set in a .env.local file and loaded
#   into the shell's environment before running migration commands. One way is to use `source .env.local`.

.PHONY: help migrate-create migrate-up sqlc-generate proto-generate

help:
	@echo "Available commands:"
	@echo "  migrate-create NAME=<name>   - Create a new SQL migration"
	@echo "  migrate-up                   - Apply all up migrations"
	@echo "  sqlc-generate                 - Generate Go code from SQL queries (requires sqlc CLI)"
	@echo "  proto-generate                - Generate Go gRPC code from protobuf definitions"

# Database migration commands using golang-migrate
# https://github.com/golang-migrate/migrate

MIGRATE_PATH=internal/db/migrations

# Creates a new set of migration files (.up.sql and .down.sql)
# Example: make migrate-create NAME=add_new_feature_table
migrate-create:
	@if [ -z "$(NAME)" ]; then \
		echo "Error: NAME is not set. Usage: make migrate-create NAME=<migration_name>"; \
		exit 1; \
	fi
	@echo ">> Creating migration files for [$(NAME)]..."
	migrate create -ext sql -dir $(MIGRATE_PATH) -seq $(NAME)

# Applies all available 'up' migrations to the database.
# The command requires the DATABASE_URL to be set in the environment.
# Usage: source .env.local && make migrate-up
migrate-up:
	@if [ -z "$$DATABASE_URL" ]; then \
		echo "Error: DATABASE_URL is not set. Please create a .env.local file and source it:"; \
		echo "  source .env.local && make migrate-up"; \
		exit 1; \
	fi
	@echo ">> Applying all 'up' migrations..."
	migrate -database "$$DATABASE_URL" -path $(MIGRATE_PATH) up

# SQL Code Generation using sqlc
# https://docs.sqlc.dev/en/latest/overview/install.html
#
# Generates type-safe Go code from SQL queries defined in internal/db/queries/
# Requires sqlc CLI to be installed. Install with: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
# Usage: make sqlc-generate
sqlc-generate:
	@if ! command -v sqlc &> /dev/null; then \
		echo "Error: sqlc CLI is not installed."; \
		echo "Install it with: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest"; \
		echo "Or see: https://docs.sqlc.dev/en/latest/overview/install.html"; \
		exit 1; \
	fi
	@echo ">> Generating Go code from SQL queries..."
	sqlc generate

# Protobuf and gRPC Code Generation
#
# This command finds all .proto files in the `proto` directory and compiles them
# into Go code using the protoc compiler. The generated files will be placed
# in the `proto` directory.
#
# Requires:
# - protoc (protobuf compiler)
# - protoc-gen-go (Go plugin for protoc)
# - protoc-gen-go-grpc (Go gRPC plugin for protoc)
proto-generate:
	@if ! which protoc > /dev/null 2>&1; then \
		echo "Error: protoc is not installed."; \
		echo "Please install the protobuf compiler. See: https://grpc.io/docs/protoc-installation/"; \
		exit 1; \
	fi
	@echo ">> Generating Go gRPC code from .proto files..."
	protoc --go_out=. --go_opt=paths=source_relative \
           --go-grpc_out=. --go-grpc_opt=paths=source_relative \
           proto/*.proto

