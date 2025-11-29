# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY ./cmd /build/cmd
COPY ./pkg /build/pkg
COPY ./examples /build/examples

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o broker ./examples/gossip-cluster

# Final stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /build/broker /broker

EXPOSE 1883 7946 7947

ENTRYPOINT ["/broker"]
