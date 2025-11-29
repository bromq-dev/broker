# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY ./cmd /build/cmd
COPY ./pkg /build/pkg
COPY ./examples /build/examples

# Build the redis-cluster example
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o broker ./examples/redis-cluster

# Final stage - distroless
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /build/broker /broker

EXPOSE 1883

ENTRYPOINT ["/broker"]
