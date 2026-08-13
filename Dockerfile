FROM golang:1.26.5-alpine3.24 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/company-service \
    ./cmd/company-service

FROM alpine:3.24

RUN apk add --no-cache ca-certificates \
    && addgroup -S company \
    && adduser -S -G company company

WORKDIR /app

COPY --from=builder --chown=company:company /out/company-service /app/company-service

USER company:company

EXPOSE 8080

ENTRYPOINT ["/app/company-service"]
