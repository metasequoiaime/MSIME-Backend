FROM --platform=$BUILDPLATFORM golang:1.25 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -trimpath -ldflags="-s -w" -o /msime-server ./cmd/msime-server
FROM debian:bookworm-slim AS native-build
RUN apt-get update && apt-get install -y --no-install-recommends build-essential cmake python3 libboost-dev libfmt-dev libspdlog-dev libsqlite3-dev nlohmann-json3-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY native ./native
COPY third_party/MSIME-Engine ./third_party/MSIME-Engine
COPY third_party/opencc ./third_party/opencc
COPY third_party/cpp-pinyin ./third_party/cpp-pinyin
RUN cmake -S native -B /build -DCMAKE_BUILD_TYPE=Release && cmake --build /build --parallel 4
FROM gcr.io/distroless/cc-debian12:nonroot
COPY --from=native-build /build/msime-engine /usr/local/bin/msime-engine
COPY --from=native-build /usr/lib/*-linux-gnu/libsqlite3.so.0* /usr/lib/
COPY third_party/opencc/LICENSE /licenses/OpenCC-LICENSE
COPY third_party/cpp-pinyin/LICENSE /licenses/cpp-pinyin-LICENSE
COPY third_party/MSIME-Engine/LICENSE /licenses/MSIME-Engine-LICENSE
COPY third_party/MSIME-Engine/NOTICE.md /licenses/MSIME-Engine-NOTICE.md
COPY --from=build /msime-server /msime-server
COPY THIRD_PARTY_NOTICES.txt /licenses/MSIME-Server-THIRD-PARTY.txt
EXPOSE 8080
ENTRYPOINT ["/msime-server", "-config", "/config/config.json"]
