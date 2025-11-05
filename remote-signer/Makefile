# Makefile for Poly-Pro Analytics Remote Signer Service
# ------------------------------------------------
# This file provides convenience commands for common development tasks,
# such as generating gRPC code from protobuf definitions.
#
# Usage:
#   make proto-generate   - Generates Go gRPC code from .proto files.
#   make help             - Shows available commands.
#
# Notes:
# - It is expected that protoc, protoc-gen-go, and protoc-gen-go-grpc are installed.

.PHONY: help proto-generate

help:
	@echo "Available commands:"
	@echo "  proto-generate   - Generate Go gRPC code from protobuf definitions"

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

