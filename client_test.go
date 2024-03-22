package gojsonclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/blizzy78/gobackoff"
	"github.com/go-json-experiment/json"
	"github.com/matryer/is"
)

type testReq struct {
	Message string `json:"message"`
}

type testRes struct {
	Reply string `json:"reply"`
}

func TestDo(t *testing.T) {
	is := is.New(t)

	reqData := testReq{
		Message: "Hello, server!",
	}

	resData := testRes{
		Reply: "Hello, client!",
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		is.Equal(req.URL.Path, "/foo")

		_ = json.MarshalWrite(writer, &resData)
	}))

	defer server.Close()

	client := New()

	req := NewRequest[*testReq, *testRes](server.URL+"/foo", http.MethodGet, &reqData)

	res, err := Do(context.Background(), client, req)
	is.NoErr(err)

	is.Equal(res.Res, &resData)
}

func TestDo_Marshal(t *testing.T) {
	is := is.New(t)

	reqData := testReq{
		Message: "Hello, server!",
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		data, _ := io.ReadAll(req.Body)
		is.Equal(string(data), reqData.Message)

		http.Error(writer, "No Content", http.StatusNoContent)
	}))

	defer server.Close()

	client := New()

	req := NewRequest(server.URL, http.MethodGet, &reqData,
		WithMarshalRequestFunc[*testReq, *testRes](func(writer io.Writer, val *testReq) error {
			_, err := writer.Write([]byte(val.Message))
			return err //nolint:wrapcheck // we don't add new info here
		}),
	)

	_, _ = Do(context.Background(), client, req)
}

func TestDo_Unmarshal(t *testing.T) {
	is := is.New(t)

	reqData := testReq{
		Message: "Hello, server!",
	}

	resData := testRes{
		Reply: "Hello, client!",
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(resData.Reply))
	}))

	defer server.Close()

	client := New()

	req := NewRequest(server.URL, http.MethodGet, &reqData,
		WithUnmarshalResponseFunc[*testReq](func(httpRes *http.Response, _ **testRes) error {
			data, _ := io.ReadAll(httpRes.Body)
			is.Equal(string(data), resData.Reply)

			return nil
		}),
	)

	_, _ = Do(context.Background(), client, req)
}

func TestDo_Method(t *testing.T) {
	is := is.New(t)

	reqData := testReq{
		Message: "Hello, server!",
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		is.Equal(req.Method, http.MethodPost)

		http.Error(writer, "No Content", http.StatusNoContent)
	}))

	defer server.Close()

	client := New()

	req := NewRequest[*testReq, *testRes](server.URL, http.MethodPost, &reqData)

	_, _ = Do(context.Background(), client, req)
}

func TestWithBaseURI(t *testing.T) {
	is := is.New(t)

	reqData := testReq{
		Message: "Hello, server!",
	}

	resData := testRes{
		Reply: "Hello, client!",
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		is.Equal(req.URL.Path, "/foo")

		_ = json.MarshalWrite(writer, &resData)
	}))

	defer server.Close()

	client := New(WithBaseURI(server.URL))

	req := NewRequest[*testReq, *testRes]("/foo", http.MethodGet, &reqData)

	_, _ = Do(context.Background(), client, req)
}

func TestDo_Retry(t *testing.T) {
	is := is.New(t)

	reqData := testReq{
		Message: "Hello, server!",
	}

	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		attempts++

		if attempts == 1 {
			http.Error(writer, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		http.Error(writer, "No Content", http.StatusNoContent)
	}))

	defer server.Close()

	client := New(withInstantBackoff())

	req := NewRequest[*testReq, *testRes](server.URL, http.MethodGet, &reqData)

	_, err := Do(context.Background(), client, req)
	is.NoErr(err)

	is.Equal(attempts, 2)
}

func TestDo_RetryFunc(t *testing.T) {
	is := is.New(t)

	reqData := testReq{
		Message: "Hello, server!",
	}

	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		attempts++

		if attempts == 1 {
			http.Error(writer, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		http.Error(writer, "No Content", http.StatusNoContent)
	}))

	defer server.Close()

	client := New(
		withInstantBackoff(),

		WithRetry(func(_ context.Context, _ *http.Response, _ error) error {
			return nil
		}),
	)

	req := NewRequest[*testReq, *testRes](server.URL, http.MethodGet, &reqData)

	_, err := Do(context.Background(), client, req)
	is.NoErr(err)

	is.Equal(attempts, 2)
}

func TestDo_RetryFunc_Abort(t *testing.T) {
	is := is.New(t)

	reqData := testReq{
		Message: "Hello, server!",
	}

	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		attempts++

		if attempts == 1 {
			http.Error(writer, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		http.Error(writer, "No Content", http.StatusNoContent)
	}))

	defer server.Close()

	httpErr := errors.New("HTTP error") //nolint:goerr113 // dynamic error is okay here

	client := New(
		withInstantBackoff(),

		WithRetry(func(_ context.Context, httpRes *http.Response, _ error) error {
			if httpRes.StatusCode < 200 || httpRes.StatusCode >= 300 {
				return httpErr
			}

			return nil
		}),
	)

	req := NewRequest[*testReq, *testRes](server.URL, http.MethodGet, &reqData)

	_, err := Do(context.Background(), client, req)

	abortErr, ok := err.(*gobackoff.AbortError) //nolint:errorlint // must be *gobackoff.AbortError
	is.True(ok)
	is.Equal(abortErr.Err, httpErr)
}

func TestDo_RetryMaxAttempts(t *testing.T) {
	is := is.New(t)

	reqData := testReq{
		Message: "Hello, server!",
	}

	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		attempts++

		http.Error(writer, "Internal Server Error", http.StatusInternalServerError)
	}))

	defer server.Close()

	client := New(
		withInstantBackoff(),
		WithMaxAttempts(5),
	)

	req := NewRequest[*testReq, *testRes](server.URL, http.MethodGet, &reqData)

	_, err := Do(context.Background(), client, req)

	_, ok := err.(*gobackoff.MaxAttemptsError) //nolint:errorlint // must be *gobackoff.MaxAttemptsError
	is.True(ok)

	is.Equal(attempts, 5)
}

func TestNewHTTPRequest_NoBody(t *testing.T) {
	is := is.New(t)

	client := New()

	req := NewRequest("", http.MethodGet, nil,
		WithMarshalRequestFunc[any, any](func(_ io.Writer, _ any) error {
			is.Fail()
			return nil
		}),
	)

	_, err := newHTTPRequest(context.Background(), client, req)
	is.NoErr(err)
}

func TestResponse_IgnoreBody(t *testing.T) {
	is := is.New(t)

	req := NewRequest("", http.MethodGet, nil,
		WithIgnoreResponseBody[any, any](),

		WithUnmarshalResponseFunc[any](func(_ *http.Response, _ *any) error {
			is.Fail()
			return nil
		}),
	)

	httpRes := http.Response{
		StatusCode: http.StatusOK,
		Status:     "OK",
		Body:       http.NoBody,
	}

	_, err := response(&httpRes, req)
	is.NoErr(err)
}

func TestResponse_NoContent(t *testing.T) {
	is := is.New(t)

	req := NewRequest("", http.MethodGet, nil,
		WithUnmarshalResponseFunc[any](func(_ *http.Response, _ *any) error {
			is.Fail()
			return nil
		}),
	)

	httpRes := http.Response{
		StatusCode: http.StatusNoContent,
		Status:     "No Content",
		Body:       http.NoBody,
	}

	_, err := response(&httpRes, req)
	is.NoErr(err)
}

func withInstantBackoff() ClientOpt {
	return WithBackoff(gobackoff.New(
		gobackoff.WithInitialDelay(1*time.Nanosecond),
		gobackoff.WithMultiplier(1.0),
		gobackoff.WithJitter(0.0),
	))
}
