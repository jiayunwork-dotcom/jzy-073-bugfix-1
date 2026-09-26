# Build: golang:1.22-alpine as requested. Static binary, no CGO.
FROM golang:1.22-alpine AS build
WORKDIR /src

# Cache dependencies first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# One command both builds the image and runs the full automated test suite
# inside the toolchain container; the image build fails if any test fails.
RUN CGO_ENABLED=0 go test ./...
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/isa-service ./cmd/server

# Minimal runtime image (no shell needed); the scratch base is only for
# execution, the toolchain base above is the required golang:1.22-alpine.
FROM scratch
COPY --from=build /out/isa-service /isa-service
EXPOSE 8080
USER 65534:65534
ENTRYPOINT ["/isa-service"]
