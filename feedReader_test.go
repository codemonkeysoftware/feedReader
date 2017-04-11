package main

import (
	"bytes"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

const (
	directoryTemplate = `<!DOCTYPE HTML><html><body><h1>Index of {{.Path}}</h1><table><tr><th>Name</th><th>Last modified</th><th>Size</th><th>Description</th></tr><tr><th colspan="4"><hr></th></tr><tr><td><a href="{{.Path}}">Parent Directory</a></td><td>&nbsp;</td><td align="right">  - </td><td>&nbsp;</td></tr>{{range .Files}}<tr><td><a href="{{.Name}}">{{.Name}}</a></td><td align="right">{{.Date}}  </td><td align="right">{{.Size}}</td><td>&nbsp;</td></tr><tr>{{end}}</table></body></html>`
	entryTimeFormat   = `02-Jan-2006 15:04`
	testZipFilePath   = "./test.zip"
)

type File struct {
	Name, Date, Size string
}

func buildFileList(fileCount int) (fl []File) {
	for i := 1; i < fileCount; i++ {
		var file File
		file.Name = "File" + strconv.Itoa(i) + ".zip"
		file.Date = time.Now().Format(entryTimeFormat)
		file.Size = "1.0M"
		fl = append(fl, file)
	}
	var file = File{
		"file.doc",
		"",
		"",
	}
	fl = append(fl, file)
	return
}

//download directory
func TestGetDirectory(t *testing.T) {
	fl := buildFileList(5)
	template, err := template.New("listing").Parse(directoryTemplate)
	if err != nil {
		t.Fatal(err)
	}
	templateData := struct {
		Path  string
		Files []File
	}{
		"",
		fl,
	}
	//serve directory listing with .zip files
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		template.Execute(w, templateData)
	}))
	defer server.Close()
	server.URL += "/"
	templateData.Path = server.URL
	//call getZipListings on directory
	downloadList, err := getZipListings(server.URL)
	//downloadList should return a slice of strings for the .zip files
	for i, v := range downloadList {
		if v != server.URL+fl[i].Name {
			t.Errorf("incorrect file name: %s", v)
		}
	}

}

func TestZipFunctions(t *testing.T) {
	//create a zip file to serve
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, testZipFilePath)
	}))
	defer server.Close()
	zipFile, err := getZip(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer zipFile.Close()
	defer os.Remove(zipFile.Name())
	if zipFile == nil {
		t.Fatal("did not open zip file")
	}
	zipReader, err := openZip(zipFile)
	if err != nil {
		t.Fatal(err)
	}
	defer zipReader.Close()
	if zipReader == nil {
		t.Fatal("did not open zip reader")
	}
	rclient, err := setupRedis()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zipReader.File {
		index, err := pushXML(f, rclient)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			t.Fatal("clear database to run tests correctly")
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		buff := bytes.NewBuffer(nil)
		_, err = io.Copy(buff, rc)
		if err != nil {
			t.Fatal(err)
		}
		rresult, err := rclient.LRange(listName, index-1, index-1).Result()
		if rresult[0] != buff.String() {
			t.Fatal("stored data not equal")
		}
	}
	t.Run("Test duplicate record", func(t *testing.T) {
		f := zipReader.File[0]
		index, err := pushXML(f, rclient)
		if err != nil {
			t.Fatal(err)
		}
		if index != 0 {
			t.Fatal("record was unique")
		}
	})
}
