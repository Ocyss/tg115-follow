FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X github.com/ocyss/tg115-follow/internal/config.Version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
    -o /out/tg115-follow ./cmd/tg115-follow

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/tg115-follow /usr/local/bin/tg115-follow
WORKDIR /data
ENTRYPOINT ["tg115-follow"]
CMD ["serve", "-config", "/data/config.toml"]
