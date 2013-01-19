package main

import (
	"flag"
	"github.com/kr/s3"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

var ServerAddr string

func forward(w http.ResponseWriter, r *http.Response) {
	for k, values := range r.Header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(r.StatusCode)
	io.Copy(w, r.Body)
}

func copyHeader(key string, request *http.Request, response *http.Response) {
	value := response.Header.Get(key)
	if value != "" {
		request.Header.Set(key, value)
	}
}

func s3Head(url string, keys s3.Keys) (*http.Response, error) {
	r, _ := http.NewRequest("HEAD", url, nil)
	r.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	s3.Sign(r, keys)
	return http.DefaultClient.Do(r)
}

func s3Put(url string, keys s3.Keys, resp *http.Response) (*http.Response, error) {
	r, _ := http.NewRequest("PUT", url, resp.Body)

	//copyHeader("Content-Type", r, resp)
	r.ContentLength = resp.ContentLength

	r.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))

	s3.Sign(r, keys)

	return http.DefaultClient.Do(r)
}

type CachingHandler struct {
	BucketName string
	Keys       s3.Keys
}

func (self *CachingHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
	}

	urlInPath := req.URL.String()[1:] // Remove the first slash in the path

	url, err := url.ParseRequestURI(urlInPath)
	if err != nil {
		http.Error(w, "Parse Error", 400)
		return
	}

	s3Url := "http://" + self.BucketName + ".s3.amazonaws.com/" + url.String()

	s3Response, err := s3Head(s3Url, self.Keys)
	if err != nil {
		log.Println(url, "Error", err)
	}

	// First fetch from cache
	if s3Response.StatusCode == 200 {
		log.Println(url, "Cache hit")
		// TODO: Redirect to signed url
		http.Redirect(w, req, s3Url, 302)
		return
	} else {
		log.Println(url, "Cache miss")
	}

	// Then try upstream
	upstreamResponse, err := http.Get(url.String())
	if upstreamResponse.StatusCode != 200 {
		log.Println(url, "Upstream error")
		forward(w, upstreamResponse)
	}

	// Store in cache
	log.Println(url, "Storing in cache")
	s3Response, err = s3Put(s3Url, self.Keys, upstreamResponse)
	if err != nil {
		log.Println(url, "Error", err)
	}

	if s3Response.StatusCode == 200 {
		log.Println(url, "Cache hit 2")
		http.Redirect(w, req, s3Url, 302)
		return
	}

	log.Println(url, "S3 store error")
	forward(w, s3Response)
}

func main() {
	cache := new(CachingHandler)

	flag.StringVar(&ServerAddr, "addr", ":8080", "HTTP [host]:port on which to listen")
	flag.StringVar(&cache.BucketName, "bucket", "", "S3 bucket where to cache the files")
	flag.StringVar(&cache.Keys.AccessKey, "access-key", "", "S3 access key ID")
	flag.StringVar(&cache.Keys.SecretKey, "secret-key", "", "S3 secret key")
	flag.Parse()

	server := &http.Server{
		Addr:    ServerAddr,
		Handler: cache,
	}

	log.Println(cache)

	log.Fatal(server.ListenAndServe())
}
