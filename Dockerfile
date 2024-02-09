FROM golang:1.21

WORKDIR /build

COPY . .
RUN go mod download

RUN go build -o backend ./cmd/backend/main.go

CMD ["./backend"]
