# Build 
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache build-base curl protobuf

RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1 \
    && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN make compile

RUN go build -o /go/bin/app video_rec_service/server/server.go

## Deploy
FROM alpine:latest

WORKDIR /root/

COPY --from=builder /go/bin/app .

EXPOSE 8080

ENTRYPOINT ["./app"]




