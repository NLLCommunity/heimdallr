FROM golang:alpine AS build

COPY . .

RUN ls -lah
RUN go mod download
RUN go build -ldflags "-s -w" -o /bin/heimdallr

FROM alpine:latest
COPY --from=build /bin/heimdallr /bin/heimdallr

CMD ["/bin/heimdallr"]