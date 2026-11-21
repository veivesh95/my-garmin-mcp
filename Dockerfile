# Run `go mod tidy` once before building so go.sum exists.
ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/garmin-mcp ./cmd/garmin-mcp

# distroless/static has CA certificates and runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/garmin-mcp /garmin-mcp
ENV MCP_TRANSPORT=http LISTEN_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/garmin-mcp"]
