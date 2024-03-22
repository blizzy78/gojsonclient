[![GoDoc](https://pkg.go.dev/badge/github.com/blizzy78/gojsonclient)](https://pkg.go.dev/github.com/blizzy78/gojsonclient)


gojsonclient
============

A Go package that provides a client for JSON/REST HTTP services, with automatic retry/backoff.

```go
import "github.com/blizzy78/gojsonclient"
```


Code example
------------

```go
type request struct {
	Message string `json:"message"`
}

type response struct {
	Reply string `json:"reply"`
}

client := New()

req := NewRequest[*request, *response]("https://www.example.com", http.MethodGet, &request{
	Message: "client",
})

res, _ := Do(context.Background(), client, req)
fmt.Println(res.Res.Reply)

// Output: Hello client!
```


License
-------

This package is licensed under the MIT license.
