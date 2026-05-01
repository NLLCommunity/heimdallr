# syntax=docker/dockerfile:1.20.0
FROM golang:1.26 AS builder

WORKDIR /usr/src/app

COPY go.mod go.sum ./
RUN go mod download

RUN go install github.com/a-h/templ/cmd/templ@v0.3.1001

COPY . .
RUN templ generate
RUN CGO_ENABLED=0 GOOS=linux \
  go build -a -installsuffix cgo -ldflags "-s -w" \
  -o heimdallrbot \
  github.com/NLLCommunity/heimdallr

FROM alpine:3.23

WORKDIR /usr/src/app

RUN mkdir -p /var/lib/heimdallr
ENV HEIMDALLR_BOT_DB=/var/lib/heimdallr/heimdallr.db
VOLUME /var/lib/heimdallr

RUN apk add --no-cache ca-certificates fuse3 sqlite tini

COPY --from=litestream/litestream:0.5 /usr/local/bin/litestream /bin/litestream
COPY --from=builder /usr/src/app/heimdallrbot /usr/src/app/bin/heimdallr
COPY --from=builder /usr/src/app/litestream.yml /usr/src/app/start.sh ./

EXPOSE 8484

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["./start.sh"]
