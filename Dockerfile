FROM golang:1.24.2 as builder
RUN apt-get update && apt-get -y install unzip wget curl git
WORKDIR /app
COPY goios-peer .

RUN go install github.com/swaggo/swag/cmd/swag@latest
RUN /go/bin/swag init --parseDependency --parseInternal
RUN go build -o peer


FROM ubuntu:24.04

RUN apt update && apt -y install libimobiledevice-utils libimobiledevice6 usbmuxd  \
       ffmpeg && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY run.sh run.sh
COPY --from=builder /app/docs /app/docs
COPY --from=builder /app/peer /app/peer

RUN chmod +x /app/peer run.sh

ENTRYPOINT ["./run.sh"]
