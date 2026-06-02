FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o pg_atropos .

FROM alpine:3.20
RUN apk add --no-cache postgresql-client
COPY --from=builder /build/pg_atropos /usr/local/bin/
USER nobody
ENTRYPOINT ["pg_atropos"]
CMD ["--help"]
