FROM golang:alpine AS build

COPY . .

RUN go mod download
RUN go build -ldflags "-s -w" -o /bin/heimdallr

FROM alpine:latest
ARG ENV=invalid
COPY _fly/litefs.yml ./
COPY _fly/litestream.${ENV}.yml /etc/litestream.yml
COPY --from=build /bin/heimdallr /bin/heimdallr
#COPY --from=flyio/litefs:0.5 /usr/local/bin/litefs /bin/litefs
COPY --from=litestream/litestream:0.3 /usr/local/bin/litestream /bin/litestream
RUN apk add ca-certificates fuse3 sqlite

ENTRYPOINT litestream replicate
