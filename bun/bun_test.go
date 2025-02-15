package bun

import (
	tu "github.com/sharat87/httpbun/test_utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestRunSuite(t *testing.T) {
	suite.Run(t, new(TSuite))
}

type TSuite struct {
	suite.Suite
	Mux http.Handler
}

func (s *TSuite) SetupSuite() {
	s.Mux = MakeBunHandler("")
}

func (s *TSuite) ExecRequest(request tu.R) (http.Response, []byte) {
	var bodyReader io.Reader
	if request.Body != "" {
		bodyReader = strings.NewReader(request.Body)
	}

	req := httptest.NewRequest(request.Method, "http://example.com/"+request.Path, bodyReader)

	for name, values := range request.Headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	resp := w.Result()
	responseBody, _ := io.ReadAll(resp.Body)

	return *resp, responseBody
}

func (s *TSuite) TestHeaders() {
	resp, body := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "headers",
		Headers: map[string][]string{
			"X-One": {"custom header value"},
			"X-Two": {"another custom header"},
		},
	})
	s.Equal(200, resp.StatusCode)
	s.Equal("application/json", resp.Header.Get("Content-Type"))
	s.Equal(map[string]interface{}{
		"headers": map[string]interface{}{
			"X-One": "custom header value",
			"X-Two": "another custom header",
		},
	}, tu.ParseJson(body))
}

func (s *TSuite) TestHeadersRepeat() {
	resp, body := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "headers",
		Headers: map[string][]string{
			"X-One": {"custom header value", "another custom header"},
		},
	})
	s.Equal(200, resp.StatusCode)
	s.Equal("application/json", resp.Header.Get("Content-Type"))
	s.Equal(map[string]interface{}{
		"headers": map[string]interface{}{
			"X-One": "custom header value,another custom header",
		},
	}, tu.ParseJson(body))
}

func (s *TSuite) TestBasicAuthWithoutCreds() {
	resp, body := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "basic-auth/scott/tiger",
	})
	s.Equal(401, resp.StatusCode)
	s.Equal("Basic realm=\"Fake Realm\"", resp.Header.Get("WWW-Authenticate"))
	s.Equal(map[string]interface{}{
		"authenticated": false,
		"user":          "",
	}, tu.ParseJson(body))
}

func (s *TSuite) TestBasicAuthWithValidCreds() {
	resp, body := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "basic-auth/scott/tiger",
		Headers: map[string][]string{
			"Authorization": {"Basic c2NvdHQ6dGlnZXI="},
		},
	})
	s.Equal(200, resp.StatusCode)
	s.Equal("", resp.Header.Get("WWW-Authenticate"))
	s.Equal(map[string]interface{}{
		"authenticated": true,
		"user":          "scott",
	}, tu.ParseJson(body))
}

func (s *TSuite) TestBasicAuthWithInvalidCreds() {
	resp, body := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "basic-auth/scott/tiger",
		Headers: map[string][]string{
			"Authorization": {"Basic c2NvdHQ6d3Jvbmc="},
		},
	})
	s.Equal(401, resp.StatusCode)
	s.Equal("Basic realm=\"Fake Realm\"", resp.Header.Get("WWW-Authenticate"))
	s.Equal(map[string]interface{}{
		"authenticated": false,
		"user":          "scott",
	}, tu.ParseJson(body))
}

func (s *TSuite) TestBearerAuthWithoutToken() {
	resp, body := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "bearer",
	})
	s.Equal(401, resp.StatusCode)
	s.Equal("Bearer", resp.Header.Get("WWW-Authenticate"))
	s.Equal(len(body), 0)
}

func (s *TSuite) TestBearerAuthWithToken() {
	resp, body := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "bearer",
		Headers: map[string][]string{
			"Authorization": {"Bearer my-auth-token"},
		},
	})
	s.Equal(200, resp.StatusCode)
	s.Equal("", resp.Header.Get("WWW-Authenticate"))
	s.Equal(map[string]interface{}{
		"authenticated": true,
		"token":         "my-auth-token",
	}, tu.ParseJson(body))
}

func (s *TSuite) TestDigestAuthWithoutCreds() {
	resp, body := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "digest-auth/auth/dave/diamond",
	})
	s.Equal(401, resp.StatusCode)
	match := regexp.MustCompile("\\bnonce=(\\S+)").FindStringSubmatch(resp.Header.Get("Set-Cookie"))
	if !s.NotEmpty(match) {
		return
	}
	nonce := match[1]
	m := regexp.MustCompile(
		"Digest realm=\"testrealm@host.com\", qop=\"auth,auth-int\", nonce=\"" + nonce + "\", opaque=\"[a-z0-9]+\", algorithm=MD5, stale=FALSE",
	).FindString(resp.Header.Get("WWW-Authenticate"))
	s.NotEmpty(m, "Unexpected value for WWW-Authenticate")
	s.Equal(len(body), 0)
}

func (s *TSuite) TestDigestAuthWitCreds() {
	resp, body := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "digest-auth/auth/dave/diamond",
		Headers: map[string][]string{
			"Cookie":        {"nonce=d9fc96d7fe39099441042eea21006d77"},
			"Authorization": {"Digest username=\"dave\", realm=\"testrealm@host.com\", nonce=\"d9fc96d7fe39099441042eea21006d77\", uri=\"/digest-auth/auth/dave/diamond\", algorithm=MD5, response=\"10c1132a06ac0de7c39a07e8553f0f14\", opaque=\"362d9b0fe6787b534eb27677f4210b61\", qop=auth, nc=00000001, cnonce=\"bb2ec71d21a27e19\""},
		},
	})
	s.Equal(200, resp.StatusCode)
	s.Empty(resp.Header.Get("Set-Cookie"))
	s.Equal(map[string]interface{}{
		"authenticated": true,
		"user":          "dave",
	}, tu.ParseJson(body))
}

func (s *TSuite) TestDigestAuthWitIncorrectUser() {
	resp, _ := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "digest-auth/auth/dave/diamond",
		Headers: map[string][]string{
			"Cookie":        {"nonce=0801ff8cf72e952e08643d2dc735231d"},
			"Authorization": {"Authorization: Digest username=\"dave2\", realm=\"testrealm@host.com\", nonce=\"0801ff8cf72e952e08643d2dc735231d\", uri=\"/digest-auth/auth/dave/diamond\", algorithm=MD5, response=\"72cdee27bacbfa650470d0428fe7c4e8\", opaque=\"74061f9b6361455b1a7a74c5b075fd98\", qop=auth, nc=00000001, cnonce=\"810eae48ae823e66\""},
		},
	})
	s.Equal(401, resp.StatusCode)
	match := regexp.MustCompile("\\bnonce=(\\S+)").FindStringSubmatch(resp.Header.Get("Set-Cookie"))
	if !s.NotEmpty(match) {
		return
	}
	nonce := match[1]
	m := regexp.MustCompile(
		"Digest realm=\"testrealm@host.com\", qop=\"auth,auth-int\", nonce=\"" + nonce + "\", opaque=\"[a-z0-9]+\", algorithm=MD5, stale=FALSE",
	).FindString(resp.Header.Get("WWW-Authenticate"))
	s.NotEmpty(m, "Unexpected value for WWW-Authenticate")
	// s.Equal(string(body), "")
}

func (s *TSuite) TestResponseHeaders() {
	resp, body := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "response-headers?one=two&three=four",
	})
	s.Equal(200, resp.StatusCode)
	s.Equal("application/json", resp.Header.Get("Content-Type"))
	s.Equal([]string{"two"}, resp.Header.Values("One"))
	s.Equal([]string{"four"}, resp.Header.Values("Three"))
	s.Equal(map[string]interface{}{
		"Content-Length": "102",
		"Content-Type":   "application/json",
		"One":            "two",
		"Three":          "four",
	}, tu.ParseJson(body))
}

func (s *TSuite) TestResponseHeadersRepeated() {
	resp, body := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "response-headers?one=two&one=four",
	})
	s.Equal(200, resp.StatusCode)
	s.Equal("application/json", resp.Header.Get("Content-Type"))
	s.Equal([]string{"two", "four"}, resp.Header.Values("One"))
	s.Equal(map[string]interface{}{
		"Content-Length": "105",
		"Content-Type":   "application/json",
		"One": []interface{}{
			"two",
			"four",
		},
	}, tu.ParseJson(body))
}

func (s *TSuite) TestDrip() {
	resp, body := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "drip?duration=1&delay=0",
	})
	s.Equal(200, resp.StatusCode)
	s.Equal(strings.Repeat("*", 10), string(body))
}

func (s *TSuite) TestIpInXForwardedFor() {
	resp, body := s.ExecRequest(tu.R{
		Method: "GET",
		Path:   "ip",
		Headers: map[string][]string{
			"X-Forwarded-For": {"12.34.56.78"},
		},
	})
	s.Equal(200, resp.StatusCode)
	s.Equal("application/json", resp.Header.Get("Content-Type"))
	s.Equal(map[string]interface{}{
		"origin": "12.34.56.78",
	}, tu.ParseJson(body))
}

func TestComputeDigestAuthResponse(t *testing.T) {
	response := computeDigestAuthResponse(
		"Mufasa",
		"Circle Of Life",
		"dcd98b7102dd2f0e8b11d0f600bfb0c093",
		"00000001",
		"0a4f113b",
		"auth",
		"GET",
		"/dir/index.html",
	)
	assert.Equal(
		t,
		"6629fae49393a05397450978507c4ef1",
		response,
	)
}
