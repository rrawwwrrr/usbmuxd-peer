# libimobiledevice Docker Image

Docker-образ для сборки и запуска библиотек `libimobiledevice` из исходного кода в Ubuntu 24.04.

## Основные компоненты

Собраны из исходников:
- libplist
- libtatsu
- libusbmuxd
- libimobiledevice
- libimobiledevice-glue
- usbmuxd

## Особенности
- Запуск `usbmuxd` с пробросом сокета через TCP
- Поддержка работы с iOS-устройствами
- Оптимизирован для интеграции с другими сервисами

## Требования
- Docker
- Доступ к USB-устройствам (требуются привилегии)

## Использование

### Запуск контейнера
```bash
docker run --rm --privileged \
  -v /dev/bus/usb:/dev/bus/usb \
  -p 27015:27015 \
  libimobiledevice