FROM golang:alpine AS build

COPY . .

RUN ls -lah
RUN go mod download
RUN go build -ldflags "-s -w" -o /bin/heimdallr

FROM alpine:latest
COPY _fly/litefs.yml ./
COPY --from=build /bin/heimdallr /bin/heimdallr
COPY --from=flyio/litefs:0.5 /usr/local/bin/litefs /bin/litefs
RUN apk add ca-certificates fuse3 sqlite

ENTRYPOINT litefs mount
