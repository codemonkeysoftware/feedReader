package main

import (
	"archive/zip"
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-redis/redis"
)

const (
	HTTPDirectory = "http://bitly.com/nuvi-plz"
	listName      = "NEWS_XML"
)

func getZipListings(directory string) (downloadList []string, err error) {
	response, err := http.Get(directory)
	if err != nil {
		return
	}
	URL := response.Request.URL.String()

	d, err := goquery.NewDocumentFromResponse(response)
	if err != nil {
		return
	}

	d.Find("td a").Each(func(i int, s *goquery.Selection) {
		name, ok := s.Attr("href")
		if !ok {
			return
		}
		correctFileType := strings.Contains(name, ".zip")
		if !correctFileType {
			return
		}
		downloadList = append(downloadList, URL+name)
	})
	return
}

func getZip(URL string) (zipFile *os.File, err error) {
	resp, err := http.Get(URL)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	zipFile, err = ioutil.TempFile("./", "downloaded")
	if err != nil {
		return
	}
	defer zipFile.Close()
	_, err = io.Copy(zipFile, resp.Body)
	return
}

func openZip(zipFile *os.File) (zipReader *zip.ReadCloser, err error) {
	zipReader, err = zip.OpenReader(zipFile.Name())
	return
}

func pushXML(f *zip.File, client *redis.Client) (int64, error) {
	rc, err := f.Open()
	if err != nil {
		log.Fatal(err)
	}
	defer rc.Close()
	buff := bytes.NewBuffer(nil)
	_, err = io.Copy(buff, rc)
	if err != nil {
		log.Fatal(err)
	}
	length, err := client.LLen(listName).Result()
	if err != nil {
		return 0, err
	}
	listItems, err := client.LRange(listName, 0, length).Result()
	if err != nil {
		return 0, err
	}
	for _, v := range listItems {
		if buff.String() == v {
			return 0, nil
		}
	}
	index, err := client.RPush(listName, buff.Bytes()).Result()
	if err != nil {
		return 0, err
	}
	return index, nil
}

func setupRedis() (client *redis.Client, err error) {
	client = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	_, err = client.Ping().Result()
	if err != nil {
		return
	}
	return
}

func main() {
	downloadList, err := getZipListings(HTTPDirectory)
	if err != nil {
		log.Print(err)
		return
	}
	rclient, err := setupRedis()
	if err != nil {
		log.Print(err)
		return
	}
	for _, v := range downloadList {
		zipFile, err := getZip(v)
		if err != nil {
			log.Print(err)
			return
		}
		defer zipFile.Close()
		defer os.Remove(zipFile.Name())
		zipReader, err := openZip(zipFile)
		if err != nil {
			log.Print(err)
			return
		}
		defer zipReader.Close()
		for _, f := range zipReader.File {
			_, err = pushXML(f, rclient)
		}
	}
}
