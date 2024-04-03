package gojsonclient_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/blizzy78/gojsonclient"
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

	client := gojsonclient.New()

	req := gojsonclient.NewRequest[*request, *response](
		server.URL+"/foo",
		http.MethodGet,
		&request{Message: "client"},
	)

	res, _ := gojsonclient.Do(context.Background(), client, req)
	fmt.Println(res.Res.Reply)

	// Output: Hello client!
}
