# unified-ci

![build state](https://ci-badges.maoer.co/badges/tengattack/unified-ci/build.svg) ![coverage state](https://ci-badges.maoer.co/badges/tengattack/unified-ci/coverage.svg)

A unified continuous integration tool for coding style check.

## Dependencies

* [androidlint](https://developer.android.com/studio/write/lint)
* [apidoc](http://apidocjs.com/)
* [cpplint](https://github.com/cpplint/cpplint)
* [eslint](https://github.com/eslint/eslint)
  - [eslint-plugin-html](https://github.com/BenoitZugmeyer/eslint-plugin-html)
* [golangci](https://github.com/golangci/golangci-lint)
* [ktlint](https://github.com/pinterest/ktlint)
* [phplint](https://github.com/tengattack/phplint)
* [scss-lint](https://github.com/brigade/scss-lint)
* [tslint](https://github.com/palantir/tslint)
* [remark](https://github.com/remarkjs/remark)

## Installation

```sh
cd path/to/workdir
go get -u github.com/tengattack/unified-ci
cp $GOPATH/src/github.com/tengattack/unified-ci/config.example.yml config.yml
# edit configuration
vi config.yml
$GOPATH/bin/unified-ci -config ./config.yml
```

## Introduction

It is used to check GitHub Pull Requests automatically, and generate
comments for Pull Requests.

It will read linter's configuration file from the root path of repository:
* `.eslintrc`: `.es`, `.esx`, `.html`, `.js`, `.jsx`, `.php`
* `.eslintrc.js`: `.html`, `.js`, `.php`
* `.scss-lint.yml`: `.css`, `.scss`
* `.tslint.json`: `.ts`, `.tsx`
* `.remarkrc`: `.md`

## Support Languages/Checks

1. Android: [androidlint](https://developer.android.com/studio/write/lint)
  - `.xml`, `.java`
2. APIDoc: [apidoc](http://apidocjs.com/)

3. C/C++: [cpplint](https://github.com/cpplint/cpplint)
  - `.cpp` ...
4. CSS, SCSS: [scss-lint](https://github.com/brigade/scss-lint)
  - `.css`, `.scss`
5. Golang: [golint](https://golang.org/x/lint/golint), [goreturns](https://github.com/sqs/goreturns), [golangci](https://github.com/golangci/golangci-lint)
  - `.go`
6. HTML: [eslint-plugin-html](https://github.com/BenoitZugmeyer/eslint-plugin-html)
  - `.html`, `.php`
7. JavaScript: [eslint](https://github.com/eslint/eslint)
  - `.es`, `.js` ...
8. Kotlin: [ktlint](https://github.com/pinterest/ktlint)
  - `.kt`
9. PHP: [phplint](https://github.com/tengattack/phplint)
  - `.php`
10. TypeScript: [tslint](https://github.com/palantir/tslint)
  - `.ts` ...
11. Markdown: [remark-lint](https://github.com/remarkjs/remark-lint), [remark-pangu](https://github.com/VincentBel/remark-pangu)
  - `.md`
