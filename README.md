# unified-ci

A unified continuous integration tool for coding style check.

## Dependencies

* [cpplint](https://github.com/cpplint/cpplint)
* [eslint](https://github.com/eslint/eslint)
  - [eslint-plugin-html](https://github.com/BenoitZugmeyer/eslint-plugin-html)
* [phplint](https://github.com/tengattack/phplint)
* [scss-lint](https://github.com/brigade/scss-lint)
* [tslint](https://github.com/palantir/tslint)

## Installation

```sh
cd path/to/workdir
go get -u github.com/tengattack/unified-ci
cp $GOPATH/src/github.com/tengattack/unified-ci/config.example.yaml config.yml
# edit configuration
vi config.yml
$GOPATH/bin/unified-ci -config ./config.yml
```

## Introduction

It can use for checking GitHub Pull Requests automatically, and generate
comments for Pull Requests.

It will read linter's configuration file from the root path of repository:
* `.eslintrc`: `.es`, `.esx`, `.html`, `.js`, `.jsx`, `.php`
* `.eslintrc.js`: `.html`, `.js`, `.php`
* `.scss-lint.yml`: `.css`, `.scss`
* `.tslint.json`: `.ts`, `.tsx`

## Support Languages

1. C/C++: [cpplint](https://github.com/cpplint/cpplint)
  - `.cpp` ...
2. CSS, SCSS: [scss-lint](https://github.com/brigade/scss-lint)
  - `.css`, `.scss`
3. Golang: [golint](https://golang.org/x/lint/golint), [goreturns](https://github.com/sqs/goreturns)
  - `.go`
4. HTML: [eslint-plugin-html](https://github.com/BenoitZugmeyer/eslint-plugin-html)
  - `.html`, `.php`
5. JavaScript: [eslint](https://github.com/eslint/eslint)
  - `.es`, `.js` ...
6. PHP: [phplint](https://github.com/tengattack/phplint)
  - `.php`
7. TypeScript: [tslint](https://github.com/palantir/tslint)
  - `.ts` ...
