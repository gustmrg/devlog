FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/devlog .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/devlog /usr/local/bin/devlog
RUN addgroup -S devlog && adduser -S -G devlog -h /data devlog && mkdir -p /data && chown devlog:devlog /data
USER devlog
EXPOSE 8080
ENTRYPOINT ["devlog"]
CMD ["serve", "--data-dir", "/data", "--config", "/etc/devlog/config.json"]
