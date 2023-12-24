package gojsonclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
		_ = json.NewDecoder(httpReq.Body).Decode(&req)

		_ = json.NewEncoder(writer).Encode(&response{
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
}
