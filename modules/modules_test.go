package modules

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"
)

const myKey string = "put your key here"

func TestGetPairs(t *testing.T) {
	jsonStr := []byte(`{"d":{"results":[{"foo":1, "Title":"title1", "MediaUrl":"url1"},{"bar":2, "Title":"title2", "MediaUrl":"url2"}]}, "__next":"https://api.datamarket.azure.com/Data.ashx/Bing/Search/Image?Query=\u0027Xbox\u0027&$skip=50"}`)

	result, err := getPairs(jsonStr)
	if err != nil {
		t.Error(err)
	} else {
		answer := []ResultPair{{Title: "title1", MediaUrl: "url1"},
			{Title: "title2", MediaUrl: "url2"}}
		for indx := 0; indx < 2; indx++ {
			if answer[indx] != result[indx] {
				msg := fmt.Sprintf("resulting JSON differs. Expected: %s, Got: %s", answer, result)
				t.Error(msg)
			}
		}
	}
}

func TestSendQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sendQuery test in short mode.")
	}
	req := Requester{queryStr: "cat", accountKey: myKey}
	qResult, err := req.sendQuery()
	if err != nil {
		t.Error(err)
	} else {
		jsonStr, _ := ioutil.ReadAll(*qResult)
		(*qResult).Close()
		t.Log(string(jsonStr))
	}
}

func TestGet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Get test in short mode.")
	}
	yahooImg := "http://k.yimg.jp/images/top/sp2/cmn/logo-ns-130528.png"
	g := Getter{url: yahooImg}
	result, err := g.Get()

	if err != nil {
		t.Error(err)
	} else {

		tmpdir := os.TempDir()
		imgPath := path.Join(tmpdir, "yahooImg.png")
		var file *os.File
		file, err = os.Create(imgPath)

		img, err := ioutil.ReadAll(*result)
		(*result).Close()
		if err != nil {
			t.Error(err)
		}
		t.Log("Wrote the image to:", imgPath)
		file.Write(img)
	}

}

func TestValidFilenameMaker(t *testing.T) {
	tmpdir := os.TempDir()
	validFile, err := validFilenameMaker(path.Join(tmpdir, "foo.png"))
	if validFile != path.Join(tmpdir, "foo.png") || err != nil {
		t.Error("validFilenameMaker failed to validate.")
	}

	os.Create(path.Join(tmpdir, "foo.png"))
	for i := 0; i < maxIndices; i++ {
		os.Create(path.Join(tmpdir, fmt.Sprintf("foo_%d.png", i)))
	}

	validFile, err = validFilenameMaker(path.Join(tmpdir, "foo.png"))
	if validFile != "" || err == nil {
		t.Error("validFilenameMaker failed to confirm an invalid filename.")
	}

	os.Remove(path.Join(tmpdir, "foo.png"))
	for i := 0; i < maxIndices; i++ {
		os.Remove(path.Join(tmpdir, fmt.Sprintf("foo_%d.png", i)))
	}
}
