# Build
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/explain ./cmd/explain

# Run
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/explain /usr/local/bin/explain
ENTRYPOINT ["/usr/local/bin/explain"]
