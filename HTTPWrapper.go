package http_wrapper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	error "github.com/abhinav-codealchemist/custom-error-go"
	"github.com/ajg/form"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	NOT_ASSIGNED                              = ""
	DEFAULT_TIMEOUT                           = 20 * time.Second
	AUTHORIZATION_TOKEN_PREFIX                = "Token"
	CONTENT_TYPE_APP_JSON         ContentType = "application/json"
	CONTENT_TYPE_FORM_URL_ENCODED ContentType = "application/x-www-form-urlencoded"
)

type ContentType string

type HttpRequestParams struct {
	endpoint      string            // the api endpoint
	method        string            // the http request method, get/post
	body          interface{}       // the request body, should be json serializable
	queryParams   map[string]string // the request query url params
	authUserName  string            // basic auth user name
	authPassword  string            // basic auth password
	basicAuth     string            // basic auth
	authToken     string            // to be used in cases where token based authentication is required
	host          string            // to specify host header
	customHeaders map[string]string // to set any custom headers, if any
	contentType   ContentType       // to set content type
	timeout       time.Duration     // to set custom request timeout if needed, default is 20 secs
}

func NewHttpRequestParams(endpoint string, method string) *HttpRequestParams {
	return &HttpRequestParams{endpoint: endpoint, method: method, body: nil, queryParams: make(map[string]string, 0), customHeaders: make(map[string]string, 0), contentType: CONTENT_TYPE_APP_JSON}
}

func (a *HttpRequestParams) SetAuthPassword(authPassword string) {
	a.authPassword = authPassword
}

func (a *HttpRequestParams) SetAuthUserName(authUserName string) {
	a.authUserName = authUserName
}

func (a *HttpRequestParams) SetAuth(username string, password string) {
	a.authPassword = password
	a.authUserName = username
}

func (a *HttpRequestParams) SetAuthToken(authToken string) {
	a.authToken = authToken
}

func (a *HttpRequestParams) SetBody(requestBody interface{}) {
	a.body = requestBody
}

func (a *HttpRequestParams) SetQueryParams(requestQueryParams map[string]string) {
	a.queryParams = requestQueryParams
}

func (a *HttpRequestParams) AddQueryParam(key, val string) {
	if a.queryParams == nil {
		a.queryParams = make(map[string]string, 0)
	}
	a.queryParams[key] = val
}

func (a *HttpRequestParams) AddHeader(key, val string) {
	if a.customHeaders == nil {
		a.customHeaders = make(map[string]string, 0)
	}
	a.customHeaders[key] = val
}

func (a *HttpRequestParams) Host() string {
	return a.host
}

func (a *HttpRequestParams) SetHost(host string) {
	a.host = host
}

func (a *HttpRequestParams) SetContentType(contentType ContentType) {
	a.contentType = contentType
}

func (a *HttpRequestParams) BasicAuth() string {
	return a.basicAuth
}

func (a *HttpRequestParams) SetBasicAuth(basicAuth string) {
	a.basicAuth = basicAuth
}

func (a *HttpRequestParams) SetTimeout(timeout time.Duration) {
	a.timeout = timeout
}

func MakeApiCallWithRetries(ctx context.Context, request *HttpRequestParams, responseAddr interface{}, retriesCount int) (customError error.CustomError) {
	for i := 0; i <= retriesCount; i++ {
		customError = MakeApiCall(ctx, request, responseAddr)
		if customError.Exists() && (customError.ErrorCode() == error.API_REQUEST_ERROR || customError.ErrorCode() == error.API_REQUEST_STATUS_ERROR) {
			continue
		} else {
			return
		}
	}
	return
}

// responseAddr is the the address of the struct to put the api response
func MakeApiCall(ctx context.Context, request *HttpRequestParams, responseAddr interface{}) (customError error.CustomError) {
	body, customError := MakeApiCallWithRawResponse(ctx, request)
	if customError.Exists() {
		return
	}
	err := json.Unmarshal(body, &responseAddr)
	if err != nil {
		customError = error.NewCustomError(error.JSON_DESERIALIZATION_ERROR, err.Error()).
			WithParam("response", string(body)).
			WithParam("request", fmt.Sprintf("%+v", request))
		customError.Log()
	}
	return customError
}

func MakeApiCallWithRawResponse(ctx context.Context, request *HttpRequestParams) (body []byte, customError error.CustomError) {
	var requestBuffer io.Reader
	if request.body != nil {
		switch request.contentType {
		case CONTENT_TYPE_FORM_URL_ENCODED:
			values, err := form.EncodeToValues(request.body)
			if err != nil {
				customError = error.NewCustomError(error.FORM_SERIALIZATION_ERROR, fmt.Sprintf("request: %+v, error: %s", request.body, err.Error()))
				customError.Log()
				return
			}
			requestBuffer = strings.NewReader(values.Encode())
		case CONTENT_TYPE_APP_JSON:
			requestJSONForm, err := json.Marshal(request.body)
			if err != nil {
				customError = error.NewCustomError(error.JSON_SERIALIZATION_ERROR, fmt.Sprintf("request: %+v, error: %s", request.body, err.Error()))
				customError.Log()
				return
			}
			requestBuffer = bytes.NewBuffer(requestJSONForm)
		}
	} else {
		requestBuffer = nil
	}

	url, err := url.Parse(request.endpoint)
	if err != nil {
		customError = error.NewCustomError(error.API_URL_PARSING_ERROR, fmt.Sprintf("url: %s; error: %s", request.endpoint, err.Error()))
		customError.Log()
		return
	}

	httpRequest, err := http.NewRequest(request.method, url.String(), requestBuffer)
	if request.queryParams != nil {
		q := httpRequest.URL.Query()
		for k, v := range request.queryParams {
			q.Add(k, v)
		}
		httpRequest.URL.RawQuery = q.Encode()
	}

	if err != nil {
		customError = error.NewCustomError(error.API_REQUEST_CREATION_ERROR, fmt.Sprintf("url: %s; error: %s", url.String(), err.Error()))
		customError.Log()
		return
	}

	client := &http.Client{Timeout: DEFAULT_TIMEOUT}
	if request.timeout != time.Duration(0) {
		client.Timeout = request.timeout
	}

	httpRequest.Header.Set("Content-Type", string(request.contentType))

	if request.authToken != NOT_ASSIGNED {
		httpRequest.Header.Set("Authorization", fmt.Sprintf("%s %s", AUTHORIZATION_TOKEN_PREFIX, request.authToken))
	}

	if request.host != NOT_ASSIGNED {
		httpRequest.Host = request.host
	}

	if request.authUserName != NOT_ASSIGNED && request.authPassword != NOT_ASSIGNED {
		httpRequest.SetBasicAuth(request.authUserName, request.authPassword)
	}

	if request.basicAuth != NOT_ASSIGNED {
		httpRequest.Header.Set("Authorization", "Basic "+request.basicAuth)
	}

	if request.customHeaders != nil {
		for k, v := range request.customHeaders {
			httpRequest.Header.Add(k, v)
		}
	}

	response, httpErr := client.Do(httpRequest)

	if httpErr != nil {
		customError = error.NewCustomError(error.API_REQUEST_ERROR, fmt.Sprintf("url: %s; error: %s", url.String(), httpErr.Error())).
			WithParam("request", fmt.Sprintf("%+v", request))
		customError.Log()
		return
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ = ioutil.ReadAll(response.Body)
		responseMap := map[string]string{}
		err = json.Unmarshal(body, &responseMap)
		customError = error.NewCustomError(error.API_REQUEST_STATUS_ERROR, fmt.Sprintf("url: %s; status code: %d; status: %s; body: %+v", url.String(), response.StatusCode, response.Status, responseMap)).
			WithParam("response", string(body)).
			WithParam("request", fmt.Sprintf("%+v", request))
		customError.WithParam("response-json", string(body))
		customError.Log()
		return
	}

	body, _ = ioutil.ReadAll(response.Body)
	return
}
