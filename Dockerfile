# Build stage
FROM golang:1.22-alpine AS build
WORKDIR /app
RUN apk add --no-cache gcc musl-dev
COPY go.mod go.sum ./ 
RUN go mod download
COPY . .
RUN go build -o server ./cmd/server

# Runtime stage
FROM alpine:3.20
WORKDIR /app
RUN adduser -D -u 10001 app
COPY --from=build /app/server /app/server
COPY web /app/web
COPY schema.pg.sql /app/schema.pg.sql
ENV ADDR=:8080

ENV SESSION_LIFETIME_HOURS=24
USER app
EXPOSE 8080
CMD ["/app/server"]
