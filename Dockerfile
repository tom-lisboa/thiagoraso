FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/mentoria-webhook ./cmd/server

FROM alpine:3.22

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=build /out/mentoria-webhook /app/mentoria-webhook

USER app
EXPOSE 8080

ENV HTTP_ADDR=:8080
CMD ["/app/mentoria-webhook"]
