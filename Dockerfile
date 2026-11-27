# Go backend image: one image carries api, worker, migrate and seed binaries;
# docker-compose picks the command per service.

FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    for c in api worker migrate seed; do \
      go build -ldflags="-s -w" -o /out/$c ./cmd/$c || exit 1; \
    done

FROM alpine:3.22
# ca-certificates: HTTPS checks; tzdata: correct timestamps; wget ships with busybox (healthcheck).
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -H -u 10001 app
COPY --from=build /out/ /usr/local/bin/
USER app
EXPOSE 8080
CMD ["api"]
