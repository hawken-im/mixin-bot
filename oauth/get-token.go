package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

var (
	clientID     = "33d79ecd-1293-47cd-b00c-10e889692a3c"
	clientSecret = "8c50cb398bf58f342e5ccf43537004485d1443f66df0241fca492df52bdbdd0a"
)

const (
	mixinOAuthURL = "https://api.mixin.one/oauth/token"
)

func main() {
	if clientID == "" || clientSecret == "" {
		fmt.Println("ERROR clientID or clientSecret is empty, exit.")
		os.Exit(1)
	}

	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/oauth", oauthHandler)

	http.ListenAndServe(":8000", nil)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	_url := "https://mixin.one/oauth/authorize?client_id=%s&scope=%s&response_type=code&return_to=%s"
	return_to := ""
	_url = fmt.Sprintf(_url, clientID, "PROFILE:READ", return_to)
	http.Redirect(w, r, _url, http.StatusFound)
}

func oauthHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	code := query.Get("code")
	if len(code) != 64 {
		fmt.Fprintf(w, "invalid code: %s", code)
		return
	}

	payload := fmt.Sprintf(
		`{"client_id": "%s", "code": "%s", "client_secret": "%s"}`,
		clientID, code, clientSecret,
	)

	client := http.Client{
		Timeout: 30 * time.Second,
	}
	req, err := http.NewRequest("POST", mixinOAuthURL, bytes.NewBufferString(payload))
	if err != nil {
		msg := fmt.Sprintf("ERROR new http request failed: %s", err)
		fmt.Printf("%s", msg)
		fmt.Fprint(w, msg)
		return
	}
	if req.Header == nil {
		req.Header = http.Header{}
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		msg := fmt.Sprintf("ERROR POST %s failed: %s", mixinOAuthURL, err)
		fmt.Printf("%s", msg)
		fmt.Fprint(w, msg)
		return
	}

	defer resp.Body.Close()
	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		msg := fmt.Sprintf("ERROR read response failed: %s", err)
		fmt.Printf("%s", msg)
		fmt.Fprint(w, msg)
		return
	}

	fmt.Fprint(w, string(content))
}
