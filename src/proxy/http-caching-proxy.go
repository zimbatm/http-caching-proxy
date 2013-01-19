package main

import (
	"flag"
	"github.com/kr/s3"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
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

func copyUrl(origURL *url.URL) (newURL *url.URL) {
	newURL = new(url.URL)
	*newURL = *origURL
	return
}

func urlToPath(url *url.URL) string {
	u := copyUrl(url)
	u.Scheme = ""
	// Ignore the starting "//" chars
	return u.String()[2:]
}

func copyHeader(key string, request *http.Request, response *http.Response) {
	value := response.Header.Get(key)
	if value != "" {
		request.Header.Set(key, value)
	}
}

func s3Head(bucket, path string, keys s3.Keys) (*http.Response, error) {
	url := bucket + "/" + path
	r, _ := http.NewRequest("HEAD", url, nil)
	r.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	s3.Sign(r, keys)
	return http.DefaultClient.Do(r)
}

func s3Put(bucket, path string, keys s3.Keys, resp *http.Response) (*http.Response, error) {
	url := bucket + "/" + path
	input := resp.Body
	contentLenght := resp.ContentLength

	// S3 doesn't support chunked encodings on upload
	if resp.ContentLength < 0 {
		tmpdir := os.TempDir()
		tmpfile, _ := ioutil.TempFile(tmpdir, "upload")
		defer tmpfile.Close()
		defer os.Remove(tmpfile.Name())

		log.Println(path, "Storing upstream to ", tmpfile.Name())

		io.Copy(tmpfile, resp.Body)
		tmpfile.Seek(0, 0)
		tmpfileInfo, _ := os.Stat(tmpfile.Name())

		input = tmpfile
		contentLenght = tmpfileInfo.Size()
	}

	r, _ := http.NewRequest("PUT", url, input)
	r.ContentLength = contentLenght

	copyHeader("Content-Type", r, resp)

	r.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))

	s3.Sign(r, keys)

	log.Println(path, "Cache push. Size:", contentLenght)

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

	bucketUrl := "http://" + self.BucketName + ".s3.amazonaws.com"
	bucketPath := urlToPath(url)
	s3Url := bucketUrl + "/" + bucketPath

	s3Response, err := s3Head(bucketUrl, bucketPath, self.Keys)
	if err != nil {
		log.Println(bucketPath, "Error", err)
	}

	// First fetch from cache
	if s3Response.StatusCode == 200 {
		log.Println(bucketPath, "Cache hit")
		// TODO: Redirect to signed url
		http.Redirect(w, req, s3Url, http.StatusMovedPermanently)
		return
	} else {
		log.Println(bucketPath, "Cache miss")
	}

	// Then try upstream
	upstreamResponse, err := http.Get(url.String())
	if upstreamResponse.StatusCode != 200 {
		log.Println(bucketPath, "Upstream error")
		forward(w, upstreamResponse)
	}

	// Store in cache
	log.Println(bucketPath, "Storing in cache")
	s3Response, err = s3Put(bucketUrl, bucketPath, self.Keys, upstreamResponse)
	if err != nil {
		log.Println(bucketPath, "Error", err)
	}

	if s3Response.StatusCode == 200 {
		log.Println(bucketPath, "Cache hit 2")
		http.Redirect(w, req, s3Url, http.StatusFound)
		return
	}

	log.Println(bucketPath, "S3 store error")
	forward(w, s3Response)
}

func main() {
	cache := new(CachingHandler)

	flag.StringVar(&ServerAddr, "addr", ":8080", "HTTP [host]:port on which to listen.")
	flag.StringVar(&cache.BucketName, "bucket", os.Getenv("S3_BUCKET_NAME"), "S3 bucket where to cache the files. ENV: S3_BUCKET_NAME")
	flag.StringVar(&cache.Keys.AccessKey, "access-key", os.Getenv("AWS_ACCESS_KEY_ID"), "S3 access key ID. ENV: AWS_ACCESS_KEY_ID")
	flag.StringVar(&cache.Keys.SecretKey, "secret-key", os.Getenv("AWS_SECRET_ACCESS_KEY"), "S3 secret key. ENV: AWS_SECRET_ACCESS_KEY")
	flag.Parse()

	server := &http.Server{
		Addr:    ServerAddr,
		Handler: cache,
	}

	log.Println("S3 access:", cache.Keys.AccessKey)
	log.Println("S3 bucket:", cache.BucketName)
	log.Println("Server listening on:", ServerAddr)

	log.Fatal(server.ListenAndServe())
}
