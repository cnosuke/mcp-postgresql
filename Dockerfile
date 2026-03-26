FROM golang:1.24-alpine as builder

# Install build dependencies
RUN apk add --no-cache make git

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application (with CGO disabled for static linking)
RUN CGO_ENABLED=0 make

# Use distroless as minimal base image
FROM gcr.io/distroless/static

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/bin/mcp-postgresql /app/bin/mcp-postgresql
COPY --from=builder /app/config.yml /app/config.yml

# Define environment variables with default values
ENV LOG_PATH=""
ENV LOG_LEVEL="info"
ENV POSTGRES_HOST="localhost"
ENV POSTGRES_PORT="5432"
ENV POSTGRES_USER="postgres"
ENV POSTGRES_PASSWORD=""
ENV POSTGRES_DATABASE="postgres"
ENV POSTGRES_SCHEMA="public"
ENV POSTGRES_SSLMODE="disable"
ENV POSTGRES_DSN=""
ENV POSTGRES_READ_ONLY="false"
ENV HTTP_HOST="0.0.0.0"
ENV HTTP_PORT="8080"
ENV HTTP_ENDPOINT="/mcp"
ENV HTTP_AUTH_TOKEN=""
ENV HTTP_ALLOWED_ORIGINS=""

# Expose HTTP port
EXPOSE 8080

# Set entrypoint (stdio transport by default; override with "http" for HTTP transport)
ENTRYPOINT ["/app/bin/mcp-postgresql", "server", "--config=/app/config.yml"]
