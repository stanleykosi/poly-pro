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

.PHONY: help migrate-create migrate-up

help:
	@echo "Available commands:"
	@echo "  migrate-create NAME=<name>   - Create a new SQL migration"
	@echo "  migrate-up                   - Apply all up migrations"

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

