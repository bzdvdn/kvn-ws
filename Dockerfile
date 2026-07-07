# @sk-task docker-ci#T1.1: multi-stage Docker build with frontend (AC-003)
# Frontend build stage
FROM node:20-alpine AS frontend
WORKDIR /app
COPY src/internal/webui/frontend/package*.json ./
RUN npm ci
COPY src/internal/webui/frontend/ .
RUN npm run build

# Go build stage
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/dist ./src/internal/webui/frontend/dist
RUN CGO_ENABLED=0 go build -o /app/bin/client ./src/cmd/client \
 && CGO_ENABLED=0 go build -o /app/bin/server ./src/cmd/server \
 && CGO_ENABLED=0 go build -o /app/bin/relay ./src/cmd/relay \
 && CGO_ENABLED=0 go build -o /app/bin/kvn-web ./src/cmd/web

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache iproute2 ca-certificates nftables
COPY --from=build /app/bin/client /usr/local/bin/client
COPY --from=build /app/bin/server /usr/local/bin/server
COPY --from=build /app/bin/relay /usr/local/bin/relay
COPY --from=build /app/bin/kvn-web /usr/local/bin/kvn-web
ENTRYPOINT ["/usr/local/bin/server"]
