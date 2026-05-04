# Stage 1: Build frontend
FROM node:20-alpine AS frontend
WORKDIR /build/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS backend
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /build/web/out ./web/out
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /admin-server .

# Stage 3: Final minimal image
FROM alpine:3.20
# postgresql16-client provides pg_dump for Phase 6.1 backups.
# docker-cli + compose plugin are used to restart services.
RUN apk add --no-cache ca-certificates docker-cli docker-cli-compose postgresql16-client
COPY --from=backend /admin-server /usr/local/bin/admin-server
EXPOSE 9100
ENTRYPOINT ["admin-server"]
