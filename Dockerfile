# @sk-task foundation#T3.3: multi-stage Docker build (AC-003)
# @sk-task docs-and-release#T5.1: fix Go version + runtime image for TUN (AC-008)
# Build stage
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app/bin/client ./src/cmd/client \
 && CGO_ENABLED=0 go build -o /app/bin/server ./src/cmd/server

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache iproute2 ca-certificates nftables
COPY --from=build /app/bin/client /usr/local/bin/client
COPY --from=build /app/bin/server /usr/local/bin/server
ENTRYPOINT ["/usr/local/bin/server"]
