# @sk-task foundation#T3.3: multi-stage Docker build (AC-003)
# Build stage
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app/bin/client ./src/cmd/client \
 && CGO_ENABLED=0 go build -o /app/bin/server ./src/cmd/server

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /app/bin/client /usr/local/bin/client
COPY --from=build /app/bin/server /usr/local/bin/server
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/usr/local/bin/server"]
