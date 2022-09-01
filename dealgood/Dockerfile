FROM golang:1.18.4-alpine

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY misc ./
COPY *.go ./

RUN go build

CMD [ "/app/dealgood", "--nogui=true", "--host=ipfs.io", "--duration=-1", "--prometheus-addr=:9090"]
