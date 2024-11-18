FROM golang:1.23 AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -a -installsuffix cgo -o server ./cmd/server/main.go

FROM alpine:latest AS certs
RUN apk --update add ca-certificates

FROM reg.wehmoen.dev/library/upx:latest AS upx
COPY --from=builder /build/server /server
RUN  upx --brute /server

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=upx /server /server
CMD ["/server"]