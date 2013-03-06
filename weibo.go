/**
 * Author: xio
 * Date: 13-2-27
 * Version: 0.02
 */
package weigo

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	// "reflect"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"
)

const (
	HTTP_GET    int = 0
	HTTP_POST   int = 1
	HTTP_UPLOAD int = 2
)

func httpCall(the_url string, method int, authorization string, params map[string]interface{}) (result map[string]interface{}, err error) {
	var url_params string
	var multipart_data *bytes.Buffer //Upload Image
	var http_url string
	var http_body string
	var content_type string
	var multipart_content_type string
	var request *http.Request
	var HTTP_METHOD string

	if method == HTTP_UPLOAD {
		the_url = strings.Replace(the_url, "https://api.", "https://upload.api.", 1)
		multipart_content_type, multipart_data, err = encodeMultipart(params)
		if err != nil {
			return
		}
	} else {
		url_params, err = encodeParams(params)
		if err != nil {
			return
		}
	}

	if method == HTTP_GET {
		http_url = fmt.Sprintf("%s?%s", the_url, url_params)
		http_body = ""
	} else {
		http_url = the_url
		http_body = url_params
	}

	switch method {
	case HTTP_GET:
		HTTP_METHOD = "GET"
	case HTTP_POST:
		HTTP_METHOD = "POST"
		content_type = "application/x-www-form-urlencoded"
	case HTTP_UPLOAD:
		HTTP_METHOD = "POST"
		content_type = multipart_content_type
	}

	client := new(http.Client)
	if method == HTTP_UPLOAD {
		request, err = http.NewRequest(HTTP_METHOD, http_url, multipart_data) //Upload
	} else {
		request, err = http.NewRequest(HTTP_METHOD, http_url, strings.NewReader(http_body)) //GET & POST
	}

	if err != nil {
		return
	}

	request.Header.Add("Accept-Encoding", "gzip")
	if method != HTTP_GET {
		request.Header.Add("Content-Type", content_type)
	}

	if authorization != "" {
		request.Header.Add("Authorization", fmt.Sprintf("OAuth2 %s", authorization))
	}

	response, err := client.Do(request) // Do Request
	if err != nil {
		return
	}
	defer response.Body.Close()

	var body string
	body, err = read_body(response)
	if err != nil {
		return
	}

	result, err = parse_json(body)
	if err != nil {
		return
	}

	if error_code, ok := result["error_code"].(float64); ok {
		err = &APIError{When: time.Now(), ErrorCode: int64(error_code), Message: result["error"].(string)}
		return
	}

	return result, nil
}

func encodeParams(params map[string]interface{}) (result string, err error) {
	if len(params) > 0 {
		values := url.Values{}
		for key, value := range params {
			switch value.(type) {
			case string:
				values.Add(key, value.(string))
			case int:
				values.Add(key, strconv.Itoa(value.(int)))
			default:
				err = errors.New("Unsupport Data Type!")
				return
			}
		}
		result = values.Encode()
	}
	return
}

func encodeMultipart(params map[string]interface{}) (multipartContentType string, multipartData *bytes.Buffer, err error) {
	if len(params) > 0 {
		multipartData = new(bytes.Buffer)
		bufferWriter := multipart.NewWriter(multipartData) // type *bytes.Buffer
		defer bufferWriter.Close()
		for key, value := range params {
			switch value.(type) {
			case string:
				bufferWriter.WriteField(key, value.(string))
			case int:
				bufferWriter.WriteField(key, string(value.(int)))
			case *os.File:
				var picdata io.Writer
				picdata, err = bufferWriter.CreateFormFile(key, value.(*os.File).Name())
				multipartContentType = bufferWriter.FormDataContentType()
				if err != nil {
					return "", nil, err
				}
				io.Copy(picdata, value.(*os.File))
			default:
				err = errors.New("Unsupport Data Type!")
				if err != nil {
					return "", nil, err
				}
			}
		}
		return
	}
	return
}

func read_body(response *http.Response) (body string, err error) {
	var reader io.ReadCloser
	var contents []byte

	using_gzip := response.Header.Get("Content-Encoding")
	switch using_gzip {
	case "gzip":
		reader, err = gzip.NewReader(response.Body)
		if err != nil {
			return
		}
		defer reader.Close()
	default:
		reader = response.Body
	}
	contents, err = ioutil.ReadAll(reader)
	if err != nil {
		return
	}
	body = string(contents)
	return body, nil
}

func parse_json(body string) (result map[string]interface{}, err error) {
	var data interface{} //Abstract Interface
	body_bytes := []byte(body)
	err = json.Unmarshal(body_bytes, &data)
	if err != nil {
		return
	}
	result = data.(map[string]interface{})
	return result, nil
}

type HttpObject struct {
	client *APIClient
	method int
}

func (http *HttpObject) Call(uri string, params map[string]interface{}) (result map[string]interface{}, err error) {
	var url = fmt.Sprintf("%s%s.json", http.client.api_url, uri)
	if http.client.is_expires() {
		err = &APIError{When: time.Now(), ErrorCode: 21327, Message: "expired_token"}
		return result, err
	}
	result, err = httpCall(url, http.method, http.client.access_token, params)
	return
}

type APIClient struct {
	app_key       string
	app_secret    string
	redirect_uri  string
	response_type string
	domain        string
	auth_url      string
	api_url       string
	version       string
	access_token  string
	expires       int64
	Get           *HttpObject
	Post          *HttpObject
	Upload        *HttpObject
}

func (api *APIClient) is_expires() bool {
	return api.access_token == "" || api.expires < time.Now().Unix()
}

func NewAPIClient(app_key, app_secret, redirect_uri, response_type string) *APIClient {
	var api = &APIClient{
		app_key:       app_key,
		app_secret:    app_secret,
		redirect_uri:  redirect_uri,
		response_type: response_type,
		domain:        "api.weibo.com",
		version:       "2",
	}

	api.auth_url = fmt.Sprintf("https://%s/oauth2/", api.domain)
	api.api_url = fmt.Sprintf("https://%s/%s/", api.domain, api.version)
	api.Get = &HttpObject{client: api, method: HTTP_GET}
	api.Post = &HttpObject{client: api, method: HTTP_POST}
	api.Upload = &HttpObject{client: api, method: HTTP_UPLOAD}
	return api
}

func (api *APIClient) SetAccessToken(access_token string, expires int64) *APIClient {
	api.access_token = access_token
	api.expires = expires
	return api
}

func (api *APIClient) GetAuthorizeUrl(params map[string]interface{}) (authorize_url string, err error) {

	var url_params = map[string]interface{}{
		"client_id":     api.app_key,
		"response_type": api.response_type,
		"redirect_uri":  api.redirect_uri,
	}

	for key, value := range params {
		url_params[key] = value
	}
	var encode_params string
	encode_params, err = encodeParams(url_params)
	if err != nil {
		return authorize_url, err
	}
	authorize_url = fmt.Sprintf("%s%s?%s", api.auth_url, "authorize", encode_params)

	return authorize_url, nil
}

func (api *APIClient) RequestAccessToken(code string) (result map[string]interface{}, err error) {
	var the_url string = fmt.Sprintf("%s%s", api.auth_url, "access_token")

	var params = map[string]interface{}{
		"client_id":     api.app_key,
		"client_secret": api.app_secret,
		"redirect_uri":  api.redirect_uri,
		"code":          code,
		"grant_type":    "authorization_code",
	}

	result, err = httpCall(the_url, HTTP_POST, "", params)
	if err != nil {
		return
	}
	return result, nil
}

type APIError struct {
	When      time.Time
	ErrorCode int64
	Message   string
}

func (err *APIError) Error() string {
	if err == nil {
		return "Error with unknown reason"
	}
	return fmt.Sprintf("APIError When: %v ErrorMessage: %v ErrorCode: %v", err.When, err.Message, err.ErrorCode)
}

func checkError(err error) {
	log.Fatal(err)
}
