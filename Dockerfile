FROM golang:1.27-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/shop ./cmd/shop

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/shop /app/shop

VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/app/shop", "-addr", ":8080", "-db", "/data/shop.db"]
