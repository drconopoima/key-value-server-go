# key-value-server-go

Key-Value server in Go, JSON file disk persistence, non-blocking concurrent operations

## Pre-requisites

You'll need to install:

- [Go](https://go.dev/doc/install)
- [K6](https://k6.io/docs/getting-started/installation/)

## Launch

Using `go`:

```bash
go run . -release
```

Send a key-value pair `{"1":"1"}`:

```bash
curl -s -X POST http://localhost:8080/key/1 -d "1"
```

Retrieve the value for key `/key/1`:

```bash
curl -s http://localhost:8080/key/1
```

Delete the value for key `/key/1`:

```bash
curl -X DELETE -s http://localhost:8080/key/1
```

## How to build

Using `go`:

```bash
go build
```

## How to test

Using `go`:

```bash
go test 
```

## How to loadtest

Using K6

```bash
k6 run --vus 200 ./testdata/k6_get_loadtest.js --duration 60s
k6 run --vus 200 ./testdata/k6_post_loadtest.js --duration 60s
k6 run --vus 200 ./testdata/k6_delete_loadtest.js --duration 60s
```
