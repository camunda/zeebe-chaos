# syntax=docker/dockerfile:1.20
FROM golang:alpine as builder
RUN apk update && apk upgrade && apk add --no-cache ca-certificates && update-ca-certificates
WORKDIR /app
COPY --link dist/zbchaos ./

FROM scratch

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/zbchaos /usr/local/bin/

ENTRYPOINT ["zbchaos"]
CMD ["worker", "--jsonLogging", "--verbose"]
