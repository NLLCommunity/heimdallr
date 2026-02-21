# syntax=docker/dockerfile:1.20.0
FROM node:24-alpine AS frontend-builder

WORKDIR /usr/src/app
COPY web-dashboard ./
RUN npm install
RUN npm run build


FROM golang:1.26 AS builder

WORKDIR /usr/src/app

COPY go.mod go.sum ./

RUN go mod download

COPY ./ .
COPY --from=frontend-builder /usr/src/app/dist ./rpcserver/frontend

RUN go generate ./...
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags "-s -w" -o heimdallr .

FROM alpine:3.23

WORKDIR /usr/src/app

RUN mkdir -p /var/lib/heimdallr
ENV HEIMDALLR_BOT_DB=/var/lib/heimdallr/heimdallr.db
VOLUME /var/lib/heimdallr

RUN apk add --no-cache ca-certificates fuse3 sqlite tini

COPY --from=litestream/litestream:0.5 /usr/local/bin/litestream /bin/litestream
COPY --from=builder /usr/src/app/heimdallr /usr/src/app/bin/heimdallr
COPY --from=builder /usr/src/app/litestream.yml /usr/src/app/start.sh ./

EXPOSE 8484

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["./start.sh"]
