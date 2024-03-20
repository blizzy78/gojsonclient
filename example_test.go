package gojsonclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/go-json-experiment/json"
)

func Example() {
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

	req := NewRequest[*request, *response](server.URL+"/foo", http.MethodGet, &request{
		Message: "client",
	})

	res, _ := Do(context.Background(), client, req)
	fmt.Println(res.Res.Reply)

	// Output: Hello client!
}
