FROM --platform=linux/amd64 golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o mcp-server .

FROM --platform=linux/amd64 alpine:3.21
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/mcp-server .

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["./mcp-server"]
