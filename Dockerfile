FROM golang:1.20-alpine AS builder

WORKDIR /build

COPY ./ .

RUN set -ex \
	&& cd /build \
	&& go build -o octopus-wechat-web

FROM alpine:latest

RUN apk add --no-cache --update --quiet --no-progress tzdata \
	&& cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
	&& echo "Asia/Shanghai" > /etc/timezone
#&& apk del --quiet --no-progress tzdata

COPY --from=builder /build/octopus-wechat-web /usr/bin/octopus-wechat-web
RUN chmod +x /usr/bin/octopus-wechat-web

WORKDIR /data

ENTRYPOINT [ "/usr/bin/octopus-wechat-web" ]
