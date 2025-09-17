FROM docker-registry.x5.ru/golang:1.25 as builder
ENV GOPROXY="https://art.x5.ru/artifactory/api/go/go"
WORKDIR /app
COPY goios-peer .

#RUN go install github.com/swaggo/swag/cmd/swag@latest
#RUN /go/bin/swag init --parseDependency --parseInternal
RUN go build -o peer


FROM docker-registry.x5.ru/debian:bullseye-slim

RUN rm -rf /etc/apt/sources.list && \
  echo "deb https://art.x5.ru/artifactory/deb-debian-remote bullseye main" >> /etc/apt/sources.list && \
  echo "deb https://art.x5.ru/artifactory/deb-debian-remote bullseye-updates main" >> /etc/apt/sources.list && \
  echo "deb https://art.x5.ru/artifactory/deb-debian-security-remote bullseye-security main" >> /etc/apt/sources.list \

COPY --from=docker-base-images-prod.x5.ru/x5-certs:latest /x5/certs/* /usr/local/share/ca-certificates/

RUN apt update -o "Acquire::https::Verify-Peer=false" -y && \
    apt install -o "Acquire::https::Verify-Peer=false" -y ca-certificates libimobiledevice-utils libimobiledevice6 usbmuxd  ffmpeg && \
    update-ca-certificates

#RUN apt update && apt -y install libimobiledevice-utils libimobiledevice6 usbmuxd  ffmpeg \
#        && rm -rf /var/lib/apt/lists/*
RUN rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY run.sh run.sh
COPY --from=builder /app/peer /app/peer
COPY ddi-15F31d.zip /files/ddi-15F31d.zip

RUN chmod +x /app/peer run.sh

ENTRYPOINT ["./run.sh"]
