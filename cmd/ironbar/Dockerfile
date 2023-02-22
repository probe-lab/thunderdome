FROM golang:1.19-alpine AS builder
RUN apk add build-base

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY *.go ./

ARG GOFLAGS
RUN go build $GOFLAGS -trimpath -mod=readonly

#-------------------------------------------------------------------

FROM alpine
MAINTAINER Ian Davis <ian.davis@protocol.ai>

COPY --from=builder /app/ironbar /app/ironbar
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

CMD [ "/app/ironbar"]
