FROM golang:1.25 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o echo-server ./main.go

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /app/echo-server .
EXPOSE 8088
CMD ["./echo-server"]
