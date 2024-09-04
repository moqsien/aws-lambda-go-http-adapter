//go:build !lambdahttpadapter.partial || (lambdahttpadapter.partial && lambdahttpadapter.alb)

package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"github.com/aws/aws-lambda-go/events"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

func convertALBRequest(ctx context.Context, event events.ALBTargetGroupRequest) (*http.Request, error) {
	q := make(url.Values)

	if len(event.MultiValueQueryStringParameters) > 0 {
		for k, values := range event.MultiValueQueryStringParameters {
			for _, v := range values {
				q.Add(k, v)
			}
		}
	} else if len(event.QueryStringParameters) > 0 {
		for k, v := range event.QueryStringParameters {
			q.Add(k, v)
		}
	}

	headers := make(http.Header)
	if event.Headers != nil {
		for k, v := range event.Headers {
			headers.Add(k, v)
		}
	}

	if event.MultiValueHeaders != nil {
		for k, values := range event.MultiValueHeaders {
			for _, v := range values {
				headers.Add(k, v)
			}
		}
	}

	host := headers.Get("X-Forwarded-Host")
	if host == "" {
		host = headers.Get("Host")
		if host == "" {
			host = "127.0.0.1"
		}
	}

	sourceIp := headers.Get("X-Forwarded-For")
	if sourceIp == "" {
		sourceIp = "127.0.0.1"
	}

	proto := headers.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "http"
	}

	rUrl := buildFullRequestURLWithProto(proto, host, event.Path, "", q.Encode())
	req, err := http.NewRequestWithContext(ctx, event.HTTPMethod, rUrl, getBody(event.Body, event.IsBase64Encoded))
	if err != nil {
		return nil, err
	}

	req.Header = headers
	req.RemoteAddr = buildRemoteAddr(sourceIp)
	req.RequestURI = req.URL.RequestURI()

	return req, nil
}

type albResponseWriter struct {
	multiValueHeaders bool
	headersWritten    bool
	contentTypeSet    bool
	contentLengthSet  bool
	headers           http.Header
	body              bytes.Buffer
	res               events.ALBTargetGroupResponse
}

func (w *albResponseWriter) Header() http.Header {
	return w.headers
}

func (w *albResponseWriter) Write(p []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(p)
}

func (w *albResponseWriter) WriteHeader(statusCode int) {
	if !w.headersWritten {
		w.headersWritten = true
		w.res.StatusCode = statusCode

		for k, values := range w.headers {
			if w.multiValueHeaders {
				w.res.MultiValueHeaders[k] = values
			} else {
				w.res.Headers[k] = strings.Join(values, ",")
			}
		}
	}
}

func handleALB(multiValueHeaders bool) func(ctx context.Context, event events.ALBTargetGroupRequest, adapter AdapterFunc) (events.ALBTargetGroupResponse, error) {
	return func(ctx context.Context, event events.ALBTargetGroupRequest, adapter AdapterFunc) (events.ALBTargetGroupResponse, error) {
		req, err := convertALBRequest(ctx, event)
		if err != nil {
			var def events.ALBTargetGroupResponse
			return def, err
		}

		w := albResponseWriter{
			multiValueHeaders: multiValueHeaders,
			headers:           make(http.Header),
			res:               events.ALBTargetGroupResponse{},
		}

		if multiValueHeaders {
			w.res.MultiValueHeaders = make(map[string][]string)
		} else {
			w.res.Headers = make(map[string]string)
		}

		if err = adapter(ctx, req, &w); err != nil {
			var def events.ALBTargetGroupResponse
			return def, err
		}

		b, err := io.ReadAll(&w.body)
		if err != nil {
			var def events.ALBTargetGroupResponse
			return def, err
		}

		if !w.contentTypeSet {
			w.res.Headers["Content-Type"] = http.DetectContentType(b)
		}

		if !w.contentLengthSet {
			w.res.Headers["Content-Length"] = strconv.Itoa(len(b))
		}

		if utf8.Valid(b) {
			w.res.Body = string(b)
		} else {
			w.res.IsBase64Encoded = true
			w.res.Body = base64.StdEncoding.EncodeToString(b)
		}

		return w.res, nil
	}
}

func NewALBHandler(adapter AdapterFunc, multiValueHeaders bool) func(context.Context, events.ALBTargetGroupRequest) (events.ALBTargetGroupResponse, error) {
	return NewHandler(handleALB(multiValueHeaders), adapter)
}
