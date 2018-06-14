package monitor

import (
	"net/http"
	"io/ioutil"
	"github.com/antchfx/htmlquery"
	"bytes"
	"golang.org/x/net/html"
	"time"
	"fmt"
	"strings"
	"log"
	"github.com/pkg/errors"
)

func NewService() Service {
	s := service{}
	go func() {
		for true {
			err := s.runTest()
			if err != nil {
				_, err = http.Post("http://hal.go2hal/api/alert/2054274878", "text/plain", strings.NewReader(err.Error()))
				if err != nil {
					log.Println(err.Error())
				}
			}

			time.Sleep(1 * time.Minute)
		}
	}()
	return s
}

type Service interface {
}

type service struct {
}

func (s *service) runTest() (err error) {
	resp, err := http.Get("http://rc20.sbic.co.za:3073/CICS/CWBA/FIDOCUSR")
	if err != nil {
		return
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	doc, err := htmlquery.Parse(bytes.NewReader(b))
	if err != nil {
		return
	}

	nodes := htmlquery.Find(doc, "//tr")
	for _, node := range nodes {
		item := node.FirstChild
		count := 0
		error := false
		row := fidoRow{}
		for item != nil {
			if item.Data == "td" {
				switch count {
				case 0:
					n := item.FirstChild.FirstChild
					for _, a := range n.Attr {
						switch a.Key {
						case "value":
							row.name = a.Val

						}

					}
				case 1:
					row.status = item.FirstChild.FirstChild.Data
					error = error || !checkBgColor(item.Attr, "#CCFFCC")
				case 2:
					row.dsa = item.FirstChild.FirstChild.Data
					error = error || !checkBgColor(item.Attr, "#CCFFCC")
				case 3:
					row.edsa = item.FirstChild.FirstChild.Data
					error = error || !checkBgColor(item.Attr, "#CCFFCC")
				case 4:
					row.mxt = item.FirstChild.FirstChild.Data
					error = error || !checkBgColor(item.Attr, "#CCFFCC")

				}
				count++
			}
			item = item.NextSibling
		}
		if error {
			s := fmt.Sprintf("*FIDO Issue Detected*\nName: %v \nStatus: %v \nDSA: %v\n,EDSA: %v \nMAx Tasts: %v", row.name, row.status, row.dsa, row.edsa, row.mxt)
			return errors.New(s)
		}
	}
	return nil

}

func checkBgColor(attrs []html.Attribute, expected string) bool {
	for _, attr := range attrs {
		if attr.Key == "bgcolor" {
			return attr.Val == expected
		}
	}
	return false
}

type fidoRow struct {
	name   string
	status string
	dsa    string
	edsa   string
	mxt    string
	error  bool
}
