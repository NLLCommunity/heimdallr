FROM golang:1.24 AS builder

WORKDIR /usr/src/app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags "-s -w" -o heimdallr .

FROM alpine:3.22

WORKDIR /usr/src/app

RUN mkdir -p /var/lib/heimdallr
ENV HEIMDALLR_BOT_DB=/var/lib/heimdallr/heimdallr.db
VOLUME /var/lib/heimdallr

RUN apk add --no-cache ca-certificates fuse3 sqlite tini

COPY --from=litestream/litestream:0.3 /usr/local/bin/litestream /bin/litestream
COPY --from=builder /usr/src/app/heimdallr /usr/src/app/bin/heimdallr
COPY --from=builder /usr/src/app/litestream.yml /usr/src/app/start.sh ./

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["./start.sh"]
