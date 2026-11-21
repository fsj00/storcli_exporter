# build
FROM golang:1.23-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/app

# runtime
FROM debian:trixie-slim

COPY --from=builder /out/app /app
COPY ./storcli/storcli64 /opt/MegaRAID/storcli/storcli64

ENTRYPOINT ["/app"]
