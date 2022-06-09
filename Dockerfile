FROM golang:1.18-alpine as builder
RUN apk add -U --no-cache ca-certificates
WORKDIR ${GOPATH}/src/github.com/chankh/serverless-proxy
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /proxy main.go

FROM busybox
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /proxy /proxy
USER 1001:1001
ENTRYPOINT ["/proxy"]