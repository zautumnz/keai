package evaluator

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/zautumnz/keai/object"
)

// Code based on github.com/kirinlabs/HttpRequest, apache 2.0 licensed

// Request is the type of a req
type Request struct {
	cli     *http.Client
	url     string
	method  string
	timeout time.Duration
	headers map[string]string
	data    interface{}
}

// Build client
func (r *Request) buildClient() *http.Client {
	if r.cli == nil {
		r.cli = &http.Client{
			Transport: http.DefaultTransport,
			Timeout:   time.Second * r.timeout,
		}
	}
	return r.cli
}

func (r *Request) setHeaders(headers map[string]string) *Request {
	if headers != nil || len(headers) > 0 {
		for k, v := range headers {
			r.headers[k] = v
		}
	}
	return r
}

// Init headers
func (r *Request) initHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "text/plain")
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
}

// Check application/json
func (r *Request) isJSON() bool {
	if len(r.headers) > 0 {
		for _, v := range r.headers {
			if strings.Contains(strings.ToLower(v), "application/json") {
				return true
			}
		}
	}
	return false
}

// Build query data
func (r *Request) buildBody(d ...interface{}) (io.Reader, error) {
	if r.method == "GET" ||
		r.method == "DELETE" ||
		len(d) == 0 ||
		(len(d) > 0 && d[0] == nil) {
		return nil, nil
	}

	switch d[0].(type) {
	case string:
		return strings.NewReader(d[0].(string)), nil
	case []byte:
		return bytes.NewReader(d[0].([]byte)), nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return bytes.NewReader(intByte(d[0])), nil
	case *bytes.Reader:
		return d[0].(*bytes.Reader), nil
	case *strings.Reader:
		return d[0].(*strings.Reader), nil
	case *bytes.Buffer:
		return d[0].(*bytes.Buffer), nil
	default:
		if r.isJSON() {
			b, err := json.Marshal(d[0])
			if err != nil {
				return nil, err
			}
			return bytes.NewReader(b), nil
		}
	}

	t := reflect.TypeOf(d[0]).String()
	if !strings.Contains(t, "map[string]interface") {
		return nil, errors.New("unsupported data type")
	}

	data := make([]string, 0)
	for k, v := range d[0].(map[string]interface{}) {
		if s, ok := v.(string); ok {
			data = append(data, fmt.Sprintf("%s=%v", k, s))
			continue
		}
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		data = append(data, fmt.Sprintf("%s=%s", k, string(b)))
	}

	return strings.NewReader(strings.Join(data, "&")), nil
}

func (r *Request) SetTimeout(d time.Duration) *Request {
	r.timeout = d
	return r
}

// Parse query for GET request
func parseQuery(url string) ([]string, error) {
	urlList := strings.Split(url, "?")
	if len(urlList) < 2 {
		return make([]string, 0), nil
	}
	query := make([]string, 0)
	for _, val := range strings.Split(urlList[1], "&") {
		v := strings.Split(val, "=")
		if len(v) < 2 {
			return make([]string, 0), errors.New("query parameter error")
		}
		query = append(query, fmt.Sprintf("%s=%s", v[0], v[1]))
	}
	return query, nil
}

// Build GET request url
func buildURL(url string, data ...interface{}) (string, error) {
	query, err := parseQuery(url)
	if err != nil {
		return url, err
	}

	if len(data) > 0 && data[0] != nil {
		t := reflect.TypeOf(data[0]).String()
		switch t {
		case "map[string]interface {}":
			for k, v := range data[0].(map[string]interface{}) {
				vv := ""
				if reflect.TypeOf(v).String() == "string" {
					vv = v.(string)
				} else {
					b, err := json.Marshal(v)
					if err != nil {
						return url, err
					}
					vv = string(b)
				}
				query = append(query, fmt.Sprintf("%s=%s", k, vv))
			}
		case "string":
			param := data[0].(string)
			if param != "" {
				query = append(query, param)
			}
		default:
			return url, errors.New("unsupported data type")
		}

	}

	list := strings.Split(url, "?")

	if len(query) > 0 {
		return fmt.Sprintf("%s?%s", list[0], strings.Join(query, "&")), nil
	}

	return list[0], nil
}

// Send http request
func (r *Request) request(
	method,
	url string,
	data ...interface{},
) (*Response, error) {
	// Build Response
	response := &Response{}

	if method == "" || url == "" {
		return nil, errors.New("parameter method and url is required")
	}

	r.url = url
	if len(data) > 0 {
		r.data = data[0]
	} else {
		r.data = ""
	}

	var (
		err  error
		req  *http.Request
		body io.Reader
	)
	r.cli = r.buildClient()

	method = strings.ToUpper(method)
	r.method = method

	if method == "GET" || method == "DELETE" {
		url, err = buildURL(url, data...)
		if err != nil {
			return nil, err
		}
		r.url = url
	}

	body, err = r.buildBody(data...)
	if err != nil {
		return nil, err
	}

	req, err = http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	r.initHeaders(req)

	resp, err := r.cli.Do(req)
	if err != nil {
		return nil, err
	}

	response.url = url
	response.resp = resp

	return response, nil
}

// Response is the type of a response
type Response struct {
	url  string
	resp *http.Response
	body []byte
}

// Body returns the body as a byte slice
func (r *Response) Body() ([]byte, error) {
	if r == nil {
		return []byte{}, errors.New("httpRequest.Response is nil")
	}

	defer r.resp.Body.Close()

	if len(r.body) > 0 {
		return r.body, nil
	}

	if r.resp == nil || r.resp.Body == nil {
		return nil, errors.New("response or body is nil")
	}

	b, err := io.ReadAll(r.resp.Body)
	if err != nil {
		return nil, err
	}
	r.body = b

	return b, nil
}

// Content returns the body as a string
func (r *Response) Content() (string, error) {
	b, err := r.Body()
	if err != nil {
		return "", nil
	}
	return string(b), nil
}

func intByte(v interface{}) []byte {
	b := bytes.NewBuffer([]byte{})
	switch x := v.(type) {
	case int:
		binary.Write(b, binary.BigEndian, int64(x))
	case int8:
		binary.Write(b, binary.BigEndian, x)
	case int16:
		binary.Write(b, binary.BigEndian, x)
	case int32:
		binary.Write(b, binary.BigEndian, x)
	case int64:
		binary.Write(b, binary.BigEndian, x)
	case uint:
		binary.Write(b, binary.BigEndian, uint64(x))
	case uint8:
		binary.Write(b, binary.BigEndian, x)
	case uint16:
		binary.Write(b, binary.BigEndian, x)
	case uint32:
		binary.Write(b, binary.BigEndian, x)
	case uint64:
		binary.Write(b, binary.BigEndian, x)
	}
	return b.Bytes()
}

func newRequest() *Request {
	r := &Request{
		timeout: 60,
		headers: map[string]string{},
	}
	return r
}

func httpClient(args ...OBJ) OBJ {
	var uri string
	var method string
	var headers map[string]string
	var body string

	switch a := args[0].(type) {
	case *object.String:
		method = a.Value
	default:
		return NewError("http client expected method as first arg!")
	}
	switch a := args[1].(type) {
	case *object.String:
		uri = a.Value
	default:
		return NewError("http client expected uri as second arg!")
	}

	if len(args) > 2 {
		switch a := args[2].(type) {
		case *object.Hash:
			headers = make(map[string]string)
			for _, pair := range a.Pairs {
				headers[pair.Key.Inspect()] = pair.Value.Inspect()
			}
		case *object.String:
			body = a.Value
		case *object.Null:
			break
		default:
			return NewError("http client expected headers or body as third arg!")
		}
	}

	if len(args) > 3 {
		switch a := args[3].(type) {
		case *object.String:
			body = a.Value
		case *object.Null:
			break
		default:
			return NewError("http client expected body as fourth arg!")
		}
	}

	req := newRequest()

	if headers != nil {
		req.setHeaders(headers)
	}

	resp, err := req.request(method, uri, body)

	if err != nil {
		return NewError2(err.Error())
	}

	// inner http.Response struct
	res := resp.resp

	bod, err := resp.Content()
	if err != nil {
		return NewError2(err.Error())
	}
	resHeaders := make(StringObjectMap)
	for k, v := range res.Header {
		resHeaders[k] = &object.String{Value: strings.Join(v, ",")}
	}
	resHeadersVal := NewHash(resHeaders)

	ret := make(StringObjectMap)
	ret["status_code"] = &object.Integer{Value: int64(res.StatusCode)}
	ret["protocol"] = &object.String{Value: res.Proto}
	ret["body"] = &object.String{Value: bod}
	ret["headers"] = resHeadersVal

	return NewHash(ret)
}

func init() {
	RegisterBuiltin("http.create_client",
		func(env *ENV, args ...OBJ) OBJ {
			return httpClient(args...)
		})
}
