# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Install dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binaries
RUN make build

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy binaries
COPY --from=builder /build/rivetqd /app/
COPY --from=builder /build/rivetqctl /app/

# Create data directory
RUN mkdir -p /app/data

EXPOSE 8080 9090

CMD ["/app/rivetqd", "--data-dir=/app/data"]
