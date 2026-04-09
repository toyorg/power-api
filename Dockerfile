FROM golang:1.26.2-alpine AS app
RUN addgroup -S go && adduser -S -u 10000 -g go go
WORKDIR /go/src/app
COPY go.mod .
COPY go.sum .
COPY main.go .
RUN CGO_ENABLED=0 go install -ldflags "-s -w -extldflags '-static'" -tags timetzdata

FROM scratch
COPY --from=app /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=app /go/bin/power-api /power-api
COPY --from=app /etc/passwd /etc/passwd
USER go
ENTRYPOINT ["/power-api"]
