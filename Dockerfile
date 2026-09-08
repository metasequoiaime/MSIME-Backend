FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /msime-server ./cmd/msime-server
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /msime-server /msime-server
COPY THIRD_PARTY_NOTICES.txt /licenses/MSIME-Server-THIRD-PARTY.txt
EXPOSE 8080
ENTRYPOINT ["/msime-server", "-config", "/config/config.json"]
