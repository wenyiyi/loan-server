FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /server ./cmd/server
RUN CGO_ENABLED=0 go build -trimpath -o /worker ./cmd/worker
FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
USER app
COPY --from=build /server /server
COPY --from=build /worker /worker
WORKDIR /app
COPY config/local.yml config/test.yml ./config/
EXPOSE 8080
ENTRYPOINT ["/server"]
