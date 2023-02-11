# Octopus WeChat web
Octopus WeChat web limb.

# Docker
* [octopus-wechat-web](https://hub.docker.com/r/lxduo/octopus-wechat-web)
```shell
docker run -d --name=octopus-wechat-web --restart=always -v octopus-wechat-web:/data lxduo/octopus-wechat-web:latest
```

# Documentation

## Configuration
* configure.yaml
```yaml
limb:

service:
  addr: ws://10.10.10.10:11111 # Required, ocotpus address
  secret: hello # Reuqired, user defined secret
  ping_interval: 30s # Optional
  send_timeout: 3m # Optional
  sync_delay: 1m # Optional
  sync_interval: 6h # Optional

log:
  level: info
```

## Feature

* Telegram → WeChat
  * [ ] Message types
    * [x] Text
    * [x] Image
    * [x] Sticker
    * [x] Video
    * [ ] Audio
    * [ ] File
    * [ ] Mention
    * [ ] Reply
    * [ ] Location
  * [ ] Redaction

* WeChat → Telegram
  * [ ] Message types
    * [x] Text
    * [x] Image
    * [ ] Sticker
    * [x] Video
    * [x] Audio
    * [x] File
    * [ ] Mention
    * [x] Reply
    * [ ] Location
  * [ ] Chat types
    * [x] Private
    * [x] Group
  * [x] Redaction
