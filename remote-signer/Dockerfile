# Build stage
# We use a specific Go version for reproducible builds
FROM golang:1.23-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go mod and sum files to download dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Generate gRPC code from .proto files.
# We assume protoc is not installed in the golang image, so we'll do this locally
# and commit the generated files. This build step is a safeguard.
# If you have a custom build image with protoc, you could uncomment the next line.
# RUN make proto-generate

# Build the application binary.
# CGO_ENABLED=0 is important for creating a static binary.
# -o /app/server specifies the output path for the binary.
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/server ./cmd/server/main.go

# Final stage
# We use a minimal alpine image to keep the final container size small
FROM alpine:latest

# Add Certificate-Authority certificates
RUN apk --no-cache add ca-certificates

# Set the working directory
WORKDIR /root/

# Copy the compiled binary from the builder stage
COPY --from=builder /app/server .

# Make the binary executable
RUN chmod +x /root/server

# Expose port (Railway will inject PORT env var dynamically)
# We still expose a default port for documentation, but Railway uses PORT env var
EXPOSE 8081

# The command to run when the container starts
# Railway will inject PORT environment variable automatically
CMD ["./server"]

