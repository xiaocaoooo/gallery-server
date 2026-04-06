FROM golang:1.26.1-bookworm AS builder
ENV HTTP_PROXY= HTTPS_PROXY= http_proxy= https_proxy= ALL_PROXY= all_proxy= NO_PROXY= no_proxy=
WORKDIR /src
COPY go.mod go.sum ./
RUN HTTP_PROXY= HTTPS_PROXY= http_proxy= https_proxy= ALL_PROXY= all_proxy= NO_PROXY= no_proxy= GOPROXY=https://proxy.golang.org,direct go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN HTTP_PROXY= HTTPS_PROXY= http_proxy= https_proxy= ALL_PROXY= all_proxy= NO_PROXY= no_proxy= CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/gallery-server ./cmd/server

FROM scratch
ENV HTTP_PROXY= HTTPS_PROXY= http_proxy= https_proxy= ALL_PROXY= all_proxy= NO_PROXY=localhost,127.0.0.1,postgres,valkey,qdrant,seaweed-master,seaweed-volume,seaweed-filer,imaginary no_proxy=localhost,127.0.0.1,postgres,valkey,qdrant,seaweed-master,seaweed-volume,seaweed-filer,imaginary
WORKDIR /app
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/gallery-server /usr/local/bin/gallery-server
COPY migrations ./migrations
COPY openapi ./openapi
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/gallery-server"]
