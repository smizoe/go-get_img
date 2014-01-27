/*
Package modules consists of the following parts:
 - a function to send a query to Bing
 - a function to spawn workers given the query result
 - a function that gets an image given a url (from a query result)
 - a function that writes content to a file
*/
package modules

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
)

type OperationStatus int

const (
	Success OperationStatus = iota
	Failure
)

const maxIndices = 100

type OperationReply struct {
	ObjType  string
	Status   OperationStatus
	ErrorMsg interface{}
}

type ImgData struct {
	name    string
	content *io.ReadCloser
}

// OuterJSON is a type to extract important information from the JSON returned by Bing;
// this type only contains one field.
type OuterJSON struct {
	D ResultsStream
}

// ResultsStream is a type to extract important information from the JSON returned by Bing;
// this carves out the array of query results (containing pairs of image url and page title) in JSON
type ResultsStream struct {
	Results []ResultPair
}

// ResultPair carves out the necessary information (url and page title) from JSON.
type ResultPair struct {
	Title    string
	MediaUrl string
}

func getPairs(jsonStr []byte) (result []ResultPair, err error) {
	var parsed OuterJSON
	err = json.Unmarshal(jsonStr, &parsed)
	if err == nil {
		result = parsed.D.Results
	}
	return
}

// Requester's role is to make a request to Bing image search,
// and sends the query result to Spawner
type Requester struct {
	supervisor  chan<- *OperationReply
	queryStr    string
	accountKey  string
	childWorker chan<- *ResultPair
}

func NewRequester(supervisor chan<- *OperationReply, query string, accKey string, childWorker chan<- *ResultPair) *Requester {
	return &Requester{supervisor, query, accKey, childWorker}
}

// Main method is the main method for Requester; it issues a query
// and sends the query result to workers.
func (r *Requester) Main() {
	defer func() {
		name := reflect.TypeOf(*r).Name()
		if rec := recover(); rec != nil {
			r.supervisor <- &OperationReply{ObjType: name, Status: Failure, ErrorMsg: rec}
		} else {
			r.supervisor <- &OperationReply{ObjType: name, Status: Success, ErrorMsg: nil}
		}
	}()

	results, err := r.Request()
	if err != nil {
		panic(err)
	}
	for _, pair := range results {
		r.childWorker <- &pair
	}

	close(r.childWorker)
}

// Request method is the main method for Requester; it issues a query,
// gets the result and returns a pair of the name and url of an image
// possibly with an error struct
func (r *Requester) Request() (pairs []ResultPair, err error) {

	resultStream, err := r.sendQuery()
	if err != nil {
		return
	}

	jsonByte, err := ioutil.ReadAll(*resultStream)
	if err != nil {
		return
	}

	pairs, err = getPairs(jsonByte)
	return
}

// sendQuery sends a query consisting of the given string (words) to Bing Image Search.
// The return value is the query result (string that consists of a JSON object).
func (r *Requester) sendQuery() (result *io.ReadCloser, err error) {
	tr := &http.Transport{
		TLSClientConfig:    &tls.Config{},
		DisableCompression: true,
	}
	client := &http.Client{Transport: tr}
	query := url.QueryEscape("'" + r.queryStr + "'")
	qp := strings.Join([]string{"$format=json", "Query=" + query}, "&")
	rootUri := "https://api.datamarket.azure.com/Bing/Search/Image"
	requestUri := rootUri + "?" + qp

	request, err := r.createNewRequest(requestUri)
	if err != nil {
		result = nil
		return
	}

	resp, err := client.Do(request)
	if err != nil {
		result = nil
		return
	}

	return (&resp.Body), nil
}

// createNewRequest is a helper function for sendQuery;
// this creates a new request given a uri and account_key.
func (r *Requester) createNewRequest(requestUri string) (request *http.Request, err error) {
	request, err = http.NewRequest("GET", requestUri, nil)
	if err == nil {
		request.SetBasicAuth(r.accountKey, r.accountKey)
	}
	return request, err
}

// Spawner's role is to create workers and to issue operations to them.
type Spawner struct {
	supervisor   chan<- *OperationReply
	targetStream <-chan *ResultPair
	outputDir    string
	maxRoutines  int
}

func NewSpawner(supervisor chan<- *OperationReply, targets <-chan *ResultPair, outputDir string, maxRoutines int) *Spawner {
	return &Spawner{supervisor, targets, outputDir, maxRoutines}
}

// Spawner's Main function generates one Writer and many Getters.
// The Getters asynchronously fetches images and send the content to the Writer.
func (s *Spawner) Main() {
	defer func() {
		name := reflect.TypeOf(*s).Name()
		if r := recover(); r != nil {
			s.supervisor <- &OperationReply{ObjType: name, Status: Failure, ErrorMsg: r}
		} else {
			s.supervisor <- &OperationReply{ObjType: name, Status: Success, ErrorMsg: nil}
		}
	}()

	var wg sync.WaitGroup
	wgp := &wg
	getAndWrite := make(chan *ImgData)
	wgp.Add(1)

	probe := make(chan bool)

	writer := Writer{s.supervisor, s.outputDir, getAndWrite}
	go writer.Main(wgp)

	pair, ok := <-s.targetStream
	currentRoutines := 0

	for ok {
		getter := Getter{s.supervisor, pair.MediaUrl, getAndWrite, probe}
		wgp.Add(1)
		go getter.Main(wgp)
		currentRoutines++

		if currentRoutines >= s.maxRoutines {
			_ = <-probe
			currentRoutines--
		}
		pair, ok = <-s.targetStream
	}
	wgp.Done()

	wgp.Wait()
	close(getAndWrite)

}

// Getter's role is to fetch a page (image) that is specified
// by the given url. Further, it reports the (exit) status of
// the GET operation.
type Getter struct {
	supervisor   chan<- *OperationReply
	url          string
	imgPost      chan<- *ImgData
	spawnerProbe chan<- bool
}

// Getter's Main function gets an image and sends a pointer to its content to the Writer.
func (g *Getter) Main(wg *sync.WaitGroup) {
	defer func() {
		name := reflect.TypeOf(*g).Name()
		if r := recover(); r != nil {
			g.supervisor <- &OperationReply{ObjType: name, Status: Failure, ErrorMsg: r}
		} else {
			g.supervisor <- &OperationReply{ObjType: name, Status: Success, ErrorMsg: nil}
		}
		wg.Done()
		g.spawnerProbe <- true
	}()

	content, err := g.Get()

	if err != nil {
		panic(err)
	}

	fileName := filepath.Base(g.url)
	g.imgPost <- &ImgData{fileName, content}
}

// Get gets resource specified by the given url and returns a pointer to
// io.ReadCloser that spits out the content.
func (g *Getter) Get() (*io.ReadCloser, error) {
	resp, err := http.Get(g.url)
	if err != nil {
		return nil, err
	} else {
		return (&resp.Body), err
	}
}

// Writer's role is to write the image fetched by Getter to the file
// in the directory dir. The image is supposed to be stored with a name
// equal to the title of the image.
type Writer struct {
	supervisor chan<- *OperationReply
	dir        string
	imgBox     <-chan *ImgData
}

func (w *Writer) Main(wg *sync.WaitGroup) {
	defer func() {
		name := reflect.TypeOf(*w).Name()
		if r := recover(); r != nil {
			w.supervisor <- &OperationReply{ObjType: name, Status: Failure, ErrorMsg: r}
		} else {
			w.supervisor <- &OperationReply{ObjType: name, Status: Success, ErrorMsg: nil}
		}
		close(w.supervisor)
	}()
	imgData, ok := <-w.imgBox
	for ok {
		w.Write(imgData.name, imgData.content)
		imgData, ok = <-w.imgBox
	}
}

// Write writes the content of ReadCloser rc to file specified by
// `w.dir + "/" + imgName`. If the file already exists, it appends
// some number to the end of the filename (before the file extension.)
func (w *Writer) Write(imgName string, rc *io.ReadCloser) error {
	candidatePath := filepath.Join(w.dir, imgName)
	validPath, err := validFilenameMaker(candidatePath)
	if err != nil {
		return err
	}

	file, err := os.Create(validPath)
	if err != nil {
		return err
	}

	img, err := ioutil.ReadAll(*rc)
	if err != nil {
		return err
	}

	_, err = file.Write(img)
	if err != nil {
		return err
	}

	return nil
}

// validFilenameMaker returns a 'valid' path where 'valid' means that
// there is no existing file specified by the returned path.
// If the function couldn't find a 'valid' path, then this returns non-nil error.
func validFilenameMaker(writePath string) (string, error) {
	if _, err := os.Stat(writePath); os.IsNotExist(err) {
		return writePath, nil
	}
	ext := filepath.Ext(writePath)
	base := filepath.Base(writePath)
	dir := filepath.Dir(writePath)
	noSuffix := strings.TrimSuffix(base, ext)

	for i := 0; i < maxIndices; i++ {
		filename := fmt.Sprintf(noSuffix+"_%d"+ext, i)
		fullPath := filepath.Join(dir, filename)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			return fullPath, nil
		}
	}

	return "", fmt.Errorf("a number (maxIndices) of files have a similar name to %s", base)
}
