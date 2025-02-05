FROM golang:1.23.6 AS builder

WORKDIR /app
RUN --mount=type=bind,target=. go mod download
RUN --mount=type=bind,target=. go mod verify
RUN --mount=type=bind,target=. go build -o /dist/felm -ldflags="-s -w" -trimpath main.go

FROM gcr.io/distroless/cc-debian12 AS runner

ENV TZ=Asia/Tokyo
WORKDIR /app

COPY --from=builder --chown=root:root /dist/felm /app/felm
STOPSIGNAL SIGINT
ENTRYPOINT ["./felm"]
