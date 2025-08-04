# Stage 1: Build
FROM golang:1.24 AS builder

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o strato-go-dyndns .

# Stage 2: Package
FROM alpine:3.18

# Install certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/strato-go-dyndns /app/strato-go-dyndns

CMD ["/app/strato-go-dyndns"]
