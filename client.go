package gojsonclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/blizzy78/gobackoff"
)

// Client is a client for JSON/REST HTTP services.
type Client struct {
	logger             *slog.Logger
	httpClient         *http.Client
	baseURI            string
	requestMiddlewares []RequestMiddlewareFunc
	requestTimeout     time.Duration
	maxAttempts        int
	retryFunc          RetryFunc
	backoff            *gobackoff.Backoff
}

// ClientOpt is a function that configures a Client.
type ClientOpt func(client *Client)

// RequestMiddlewareFunc is a function that modifies an HTTP request.
type RequestMiddlewareFunc func(req *http.Request) error

// RetryFunc is a function that decides whether to retry an HTTP request.
// Depending on the outcome of the previous attempt, httpRes and/or err may be nil.
// A new attempt is made if the function returns a nil error.
type RetryFunc func(ctx context.Context, httpRes *http.Response, err error) error

// Request represents a JSON/REST HTTP request.
type Request[Req any, Res any] struct {
	client                *Client
	uri                   string
	method                string
	req                   Req
	marshalRequestFunc    MarshalJSONFunc[Req]
	unmarshalResponseFunc UnmarshalJSONFunc[Res]
}

// RequestOpt is a function that configures a Request.
type RequestOpt[Req any, Res any] func(req *Request[Req, Res])

// MarshalJSONFunc is a function that encodes a value to JSON and outputs it to writer.
type MarshalJSONFunc[T any] func(writer io.Writer, val T) error

// UnmarshalJSONFunc is a function that decodes JSON from httpRes.Body and stores it in val.
type UnmarshalJSONFunc[T any] func(httpRes *http.Response, val *T) error

// Response represents a JSON/REST HTTP response.
type Response[T any] struct {
	// Res is the value decoded from the response body.
	// Res will be the default value of T if StatusCode==http.StatusNoContent, or if T is Void or *Void.
	Res T

	// StatusCode is the HTTP response status code.
	StatusCode int

	// Status is the HTTP response status.
	Status string
}

// Void can be used as a request type to indicate that the request has no body,
// or as a response type to indicate that the response has no body.
type Void struct{}

type httpError string

var _ error = httpError("")

// New creates a new Client with the given options.
// The default options are: slog.Default() as the logger, http.DefaultClient as the HTTP client,
// request timeout of 30s, maximum number of attempts of 5, gobackoff.New() as the backoff,
// and a retry function that returns an error if the HTTP response status code is http.StatusBadRequest.
func New(opts ...ClientOpt) *Client {
	client := Client{
		logger:         slog.Default(),
		httpClient:     http.DefaultClient,
		requestTimeout: 30 * time.Second,
		maxAttempts:    5,
		backoff:        gobackoff.New(),

		retryFunc: func(_ context.Context, httpRes *http.Response, _ error) error {
			if httpRes != nil && httpRes.StatusCode == http.StatusBadRequest {
				return httpError(httpRes.Status)
			}

			return nil
		},
	}

	for _, opt := range opts {
		opt(&client)
	}

	return &client
}

// WithLogger configures a Client to use logger.
func WithLogger(logger *slog.Logger) ClientOpt {
	return func(client *Client) {
		client.logger = logger
	}
}

// WithHTTPClient configures a Client to use httpClient to make requests.
func WithHTTPClient(httpClient *http.Client) ClientOpt {
	return func(client *Client) {
		client.httpClient = httpClient
	}
}

// WithBaseURI configures a Client to use baseURI as the URI prefix for all requests.
func WithBaseURI(baseURI string) ClientOpt {
	return func(client *Client) {
		client.baseURI = baseURI
	}
}

// WithRequestMiddleware configures a Client to use fun as a request middleware.
// Any number of request middlewares can be added.
func WithRequestMiddleware(fun RequestMiddlewareFunc) ClientOpt {
	return func(client *Client) {
		client.requestMiddlewares = append(client.requestMiddlewares, fun)
	}
}

// WithRequestTimeout configures a Client to use timeout for each HTTP request made.
func WithRequestTimeout(timeout time.Duration) ClientOpt {
	return func(client *Client) {
		client.requestTimeout = timeout
	}
}

// WithMaxAttempts configures a Client to make at most max attempts for each request.
func WithMaxAttempts(max int) ClientOpt {
	if max < 1 {
		panic("max must be >=1")
	}

	return func(client *Client) {
		client.maxAttempts = max
	}
}

// WithRetry configures a Client to use retry as the retry function.
func WithRetry(retry RetryFunc) ClientOpt {
	if retry == nil {
		panic("retry must not be nil")
	}

	return func(client *Client) {
		client.retryFunc = retry
	}
}

// WithBackoff configures a Client to use backoff.
func WithBackoff(backoff *gobackoff.Backoff) ClientOpt {
	return func(client *Client) {
		client.backoff = backoff
	}
}

// NewRequest creates a new Request with the given client, URI, method, request data, and options.
func NewRequest[Req any, Res any](client *Client, uri string, method string, req Req, opts ...RequestOpt[Req, Res]) *Request[Req, Res] {
	request := Request[Req, Res]{
		client: client,
		uri:    uri,
		method: method,
		req:    req,

		marshalRequestFunc: func(writer io.Writer, val Req) error {
			return json.NewEncoder(writer).Encode(val) //nolint:wrapcheck // we don't add new info here
		},

		unmarshalResponseFunc: func(httpRes *http.Response, val *Res) error {
			return json.NewDecoder(httpRes.Body).Decode(val) //nolint:wrapcheck // we don't add new info here
		},
	}

	for _, opt := range opts {
		opt(&request)
	}

	return &request
}

// WithMarshalRequestFunc configures a Request to use fun as the marshal function.
func WithMarshalRequestFunc[Req any, Res any](fun MarshalJSONFunc[Req]) RequestOpt[Req, Res] {
	return func(req *Request[Req, Res]) {
		req.marshalRequestFunc = fun
	}
}

// WithUnmarshalResponseFunc configures a Request to use fun as the unmarshal function.
func WithUnmarshalResponseFunc[Req any, Res any](fun UnmarshalJSONFunc[Res]) RequestOpt[Req, Res] {
	return func(req *Request[Req, Res]) {
		req.unmarshalResponseFunc = fun
	}
}

// Do executes req and returns the response.
//
// If an HTTP request fails, it is retried using backoff according to the retry function, up to the
// maximum number of attempts.
// If the context is canceled, or if the retry function returns a non-nil error, Do stops and returns
// a gobackoff.AbortError.
//
// If Req is Void or *Void, the request will be made without a body.
// If the response status code is http.StatusNoContent, Response.Res will be the default value of Res.
// If Res is Void or *Void, the response body will be ignored, and Response.Res will be the default
// value of Res.
//
// Do is safe to call concurrently with the same Request.
func Do[Req any, Res any](ctx context.Context, req *Request[Req, Res]) (*Response[Res], error) {
	var res *Response[Res]

	err := req.client.backoff.Do(ctx, func(ctx context.Context) error {
		var (
			httpRes *http.Response
			err     error
		)

		res, httpRes, err = do(ctx, req) //nolint:bodyclose // body is already closed
		if errors.Is(err, context.Canceled) {
			return &gobackoff.AbortError{
				Err: err,
			}
		}

		if retryErr := req.client.retryFunc(ctx, httpRes, err); retryErr != nil {
			return &gobackoff.AbortError{
				Err: retryErr,
			}
		}

		return err
	}, req.client.maxAttempts)

	if err != nil {
		return nil, err //nolint:wrapcheck // we don't add new info here
	}

	return res, nil
}

func do[Req any, Res any](ctx context.Context, req *Request[Req, Res]) (*Response[Res], *http.Response, error) {
	httpReq, err := newHTTPRequest(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("new HTTP request: %w", err)
	}

	attempt := gobackoff.AttemptFromContext(ctx)

	req.client.logger.InfoContext(ctx, "execute HTTP request",
		slog.Group("request",
			slog.String("uri", httpReq.URL.String()),
			slog.String("method", httpReq.Method),
		),
		slog.Int("attempt", attempt),
	)

	ctx, cancel := context.WithTimeout(ctx, req.client.requestTimeout) //nolint:ineffassign,staticcheck // better be safe than sorry
	defer cancel()

	httpRes, err := req.client.httpClient.Do(httpReq)
	if err != nil {
		return nil, httpRes, fmt.Errorf("execute HTTP request: %w", err)
	}

	defer httpRes.Body.Close() //nolint:errcheck // we're only reading

	res, err := response(httpRes, req)
	if err != nil {
		return nil, httpRes, fmt.Errorf("get response: %w", err)
	}

	return res, httpRes, nil
}

func newHTTPRequest[Req any, Res any](ctx context.Context, req *Request[Req, Res]) (*http.Request, error) {
	var jsonReqData io.Reader = http.NoBody

	switch any(req.req).(type) {
	case Void:
	case *Void:

	default:
		buf := bytes.Buffer{}

		if err := req.marshalRequestFunc(&buf, req.req); err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}

		jsonReqData = &buf
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.method, req.client.baseURI+req.uri, jsonReqData)
	if err != nil {
		return nil, fmt.Errorf("new HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json; charset=UTF-8")
	httpReq.Header.Set("Accept", "application/json")

	for _, m := range req.client.requestMiddlewares {
		if err = m(httpReq); err != nil {
			return nil, fmt.Errorf("request middleware: %w", err)
		}
	}

	return httpReq, nil
}

func response[Req any, Res any](httpRes *http.Response, req *Request[Req, Res]) (*Response[Res], error) {
	if httpRes.StatusCode == http.StatusNoContent {
		return &Response[Res]{
			StatusCode: httpRes.StatusCode,
			Status:     httpRes.Status,
		}, nil
	}

	var jsonRes Res

	switch any(jsonRes).(type) {
	case Void:
	case *Void:
		return &Response[Res]{
			StatusCode: httpRes.StatusCode,
			Status:     httpRes.Status,
		}, nil
	}

	if err := req.unmarshalResponseFunc(httpRes, &jsonRes); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &Response[Res]{
		Res:        jsonRes,
		StatusCode: httpRes.StatusCode,
		Status:     httpRes.Status,
	}, nil
}

// BasicAuth returns a request middleware that sets the request's Authorization header to use
// HTTP Basic authentication with the provided username and password.
func BasicAuth(login string, password string) RequestMiddlewareFunc {
	return func(req *http.Request) error {
		req.SetBasicAuth(login, password)
		return nil
	}
}

// BearerAuth returns a request middleware that sets the request's Authorization header to use
// HTTP Bearer authentication with the provided token. The token will be inserted verbatim and
// may need to be encoded first.
func BearerAuth(token string) RequestMiddlewareFunc {
	return func(req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

// Error implements error.
func (e httpError) Error() string {
	return string(e)
}
