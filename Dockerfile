FROM php:7.2-fpm-alpine
LABEL maintainer="Igor Lazarev <strider2038@yandex.ru>"

ENV APP_ENV=prod \
    SWAGGER_MOCK_SPECIFICATION_URL='' \
    SWAGGER_MOCK_CACHE_TTL='0' \
    SWAGGER_MOCK_CACHE_STRATEGY='DISABLED'

RUN set -xe \
    && apk --no-cache add --update \
        nginx \
        supervisor \
    && rm -rf /usr/local/etc/php-fpm.d/* \
    && mkdir -p /var/run

WORKDIR /app

COPY .docker/ /
COPY . /app

RUN php /app/bin/console cache:clear \
    && php /app/bin/console cache:warmup

EXPOSE 80

CMD ["/usr/bin/supervisord", "--configuration", "/etc/supervisord.conf"]
