FROM golang:1.24.2 as builder
RUN apt-get update && apt-get -y install unzip wget curl git
WORKDIR /app
COPY goios-peer goios-peer

WORKDIR /app/goios-peer
RUN go install github.com/swaggo/swag/cmd/swag@latest

RUN go build -o ../peer
RUN /go/bin/swag init --parseDependency --parseInternal
RUN chmod +x ../peer

FROM ubuntu:24.04
RUN apt-get update && apt-get install -y git build-essential pkg-config autoconf automake \
    libtool-bin python3 python-is-python3 libssl-dev libusb-1.0-0-dev  \
     socat checkinstall curl libcurl4-openssl-dev net-tools vim

RUN apt update && apt -y install unzip wget curl libimobiledevice-utils libimobiledevice6 usbmuxd iproute2 net-tools \
        curl git build-essential libssl-dev zlib1g-dev socat && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY run.sh run.sh
COPY --from=builder /app/goios-peer/docs /app/docs
COPY --from=builder /app/peer /app/peer
RUN chmod +x run.sh

ENTRYPOINT ["./run.sh"]
