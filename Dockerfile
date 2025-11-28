# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o broker ./cmd/broker

# Final stage - distroless
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /build/broker /broker

EXPOSE 1883

ENTRYPOINT ["/broker"]
