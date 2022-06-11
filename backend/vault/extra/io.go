package extra

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
)

// DummyReader reads a fixed number of dummy bytes, e.g. dots; placeholder.
type DummyReader struct {
	N int64 // max
	C byte  // char to use
	i int64 // current
}

// Read reports reads, but does nothing.
func (r *DummyReader) Read(p []byte) (n int, err error) {
	if r.N == 0 {
		return 0, io.EOF
	}
	if r.N < 0 {
		return 0, fmt.Errorf("N must be positive")
	}
	for i := range p {
		p[i] = r.C
	}
	l := int64(len(p))
	if r.i+l > r.N {
		// https://i.imgur.com/2Zm3WHd.png
		s := fmt.Sprintf("%d", r.N)
		ls := int64(len(s))
		if r.N-r.i-2-ls > 0 {
			for i, c := range s {
				p[r.N-r.i-2-(ls-int64(i))] = byte(c)
			}
		}
		p[r.N-r.i-1] = 0x0a
		return int(r.N - r.i), io.EOF
	}
	r.i += l
	return len(p), nil
}

// newUploadRequest sets up a upload request; via: "Creates a new file upload
// http request with optional extra params";
// https://gist.github.com/mattetti/5914158/f4d1393d83ebedc682a3c8e7bdc6b49670083b84.
func newUploadRequest(uri string, params map[string]string, paramName, path string) (*http.Request, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	fileContents, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}
	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}
	file.Close()
	var (
		body   = new(bytes.Buffer)
		writer = multipart.NewWriter(body)
	)
	part, err := writer.CreateFormFile(paramName, fi.Name())
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(fileContents); err != nil {
		return nil, err
	}
	for key, val := range params {
		_ = writer.WriteField(key, val)
	}
	err = writer.Close()
	if err != nil {
		return nil, err
	}
	return http.NewRequest("POST", uri, body)
}
