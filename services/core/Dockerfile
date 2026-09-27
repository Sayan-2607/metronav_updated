FROM golang:1.22-alpine AS build
WORKDIR /src
COPY services/core/go.mod services/core/go.sum ./
RUN go mod download
COPY services/core/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/metronav-core ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/metronav-core /metronav-core
COPY data/network.json /data/network.json
ENV NETWORK_FILE=/data/network.json ADDR=:8080
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/metronav-core"]
