FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 is the payoff for choosing modernc.org/sqlite over mattn's cgo
# driver: the binary is fully static, so the runtime image needs no gcc, no
# libsqlite3 and no glibc.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.21

RUN apk add --no-cache sqlite \
	&& adduser -D -H -u 10001 gotic \
	&& mkdir -p /data \
	&& chown gotic:gotic /data

COPY --from=build /out/server /usr/local/bin/server

VOLUME /data

ENV ADDR=:8080 \
	DB_PATH=/data/gotic.db

EXPOSE 8080
USER gotic

ENTRYPOINT ["/usr/local/bin/server"]
