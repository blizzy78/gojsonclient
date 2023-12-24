[![GoDoc](https://pkg.go.dev/badge/github.com/blizzy78/gojsonclient)](https://pkg.go.dev/github.com/blizzy78/gojsonclient)


gojsonclient
============

A Go package that provides a client for JSON/REST HTTP services.

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

server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, httpReq *http.Request) {
	var req *request
	_ = json.UnmarshalRead(httpReq.Body, &req)

	_ = json.MarshalWrite(writer, &response{
		Reply: "Hello " + req.Message + "!",
	})
}))

defer server.Close()

client := New()

req := NewRequest[*request, *response](client, server.URL+"/foo", http.MethodGet, &request{
	Message: "client",
})

res, _ := Do(context.Background(), req)
fmt.Println(res.Res.Reply)

// Output: Hello client!
```


License
-------

This package is licensed under the MIT license.
