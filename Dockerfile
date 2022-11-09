FROM golang:1.19.3-alpine3.16 AS builder

ENV APP_HOME /go/src/TimotheFCN/LDV_RollcallBot
WORKDIR "$APP_HOME"
COPY / .
ENV CGO_ENABLED=0

RUN go mod download
RUN go mod verify
RUN go build -o rollcallbot


FROM alpine:latest
RUN apk add --no-cache tzdata
ENV TZ=Europe/Paris
WORKDIR /root/
COPY --from=builder "/go/src/TimotheFCN/LDV_RollcallBot/rollcallbot" ./
CMD ["./rollcallbot"]