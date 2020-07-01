FROM golang:1.13-alpine3.10 AS builder

ARG version

ENV GO111MODULES=on GOPROXY=https://goproxy.io,direct

# Download packages from aliyun mirrors
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
  && apk --update add --no-cache ca-certificates tzdata git openssh-client build-base

COPY go.mod go.sum /go/src/github.com/tengattack/unified-ci/

RUN cd /go/src/github.com/tengattack/unified-ci/ && \
    go mod download

COPY . /go/src/github.com/tengattack/unified-ci/

RUN cd /go/src/github.com/tengattack/unified-ci/ && \
    go install -ldflags "-X main.Version=$version" && \
    /go/bin/unified-ci -version

FROM golang:1.13-alpine3.10

ENV GO111MODULES=on GOPROXY=https://goproxy.cn,direct

# Download packages from aliyun mirrors
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
  && apk --update add --no-cache ca-certificates tzdata git openssh-client

RUN adduser -D ci && \
    chmod a+rx /home/ci && \
# Download dependencies
    apk add --no-cache libstdc++ su-exec nodejs npm php7 php7-tokenizer php7-yaml composer python3 ruby rsync && \
    node --version && \
    php --version && \
    python3 --version && \
    ruby --version && \
    \
# clang-format
    wget https://github.com/tengattack/clang-alpine/releases/download/v1.0/clang-format-9.0-alpine3.10.tar.gz && \
    tar xzf clang-format-9.0-alpine3.10.tar.gz -C /usr && \
# oclint
#    wget https://github.com/tengattack/clang-alpine/releases/download/v1.0/oclint-0.14-alpine3.10.tar.gz && \
#    tar xzf oclint-0.14-alpine3.10.tar.gz -C /opt && \
# htmllint, remarklint
    npm --registry=https://registry.npm.taobao.org \
      --cache=$HOME/.npm/.cache/cnpm \
      --disturl=https://npm.taobao.org/dist \
      --userconfig=$HOME/.cnpmrc \
      i -g apidoc@0.19.0 htmllint/htmllint-cli remark-cli vfile-reporter-json && \
    sh -c "apidoc -v --simulate || exit 0" && \
    htmllint --version && \
    remark --version && \
# phplint
# https://github.com/tengattack/phplint
    composer config -g repo.packagist composer https://packagist.phpcomposer.com && \
    composer global require tengattack/phplint && \
    rm -rf ~/.composer/cache && \
    mv ~/.composer /home/ci/ && \
    chown -R ci:ci /home/ci/.composer && \
    ln -s /home/ci/.composer/vendor/bin/phplint /usr/local/bin/ && \
    phplint | head -1 && \
# cpplint
    pip3 install cpplint && \
    cpplint --version && \
# scss-lint
# https://github.com/sds/scss-lint
    apk add --no-cache --virtual .build-deps build-base ruby-dev && \
    gem install json scss_lint --no-rdoc --no-ri && \
    scss-lint --version && \
    apk del --no-network .build-deps && \
    rm -rf ~/.gem/ && \
# golangci-lint
    wget -O- -nv https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v1.24.0 && \
    ln -s /go/bin/golangci-lint /usr/local/bin/ && \
    golangci-lint --version

COPY --from=builder /go/bin/unified-ci /unified-ci

WORKDIR /home/ci
ENTRYPOINT ["su-exec", "ci", "/unified-ci"]
