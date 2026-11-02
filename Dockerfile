FROM golang:1.25-alpine AS builder

ENV GOTOOLCHAIN=auto
WORKDIR /src
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN mkdir -p /out /tmp/go-cache /tmp/go-tmp \
	&& GOCACHE=/tmp/go-cache GOTMPDIR=/tmp/go-tmp CGO_ENABLED=0 GOOS=linux go build -o /out/nandos-cache .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/nandos-cache /app/nandos-cache

EXPOSE 8080
ENTRYPOINT ["/app/nandos-cache"]
