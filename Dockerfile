FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=${VERSION}" -o /mcp-server-abap .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates \
 && addgroup -S app && adduser -S app -G app
COPY --from=builder /mcp-server-abap /usr/local/bin/mcp-server-abap
USER app
ENTRYPOINT ["mcp-server-abap"]
