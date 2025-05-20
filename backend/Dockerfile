FROM golang:1.24.3

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY main.go .

CMD ["sh", "-c", "go run main.go 2>&1"]
