package monitor

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/antchfx/htmlquery"
	"github.com/pkg/errors"
	"github.com/tebeka/selenium"
	"github.com/weAutomateEverything/go2hal/remoteTelegramCommands"
	"golang.org/x/net/html"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func NewService(client remoteTelegramCommands.RemoteCommandClient) Service {
	s := service{
		client: client,
	}
	go func() {
		s.registerRemoteCommand()
	}()
	go func() {
		for true {
			err := s.runTest()
			if err != nil {
				s.sendScreenshot()
			}

			time.Sleep(1 * time.Minute)
		}
	}()
	return s
}

type Service interface {
}

type service struct {
	client remoteTelegramCommands.RemoteCommandClient
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
		for item != nil {
			if item.Data == "td" {
				switch count {
				case 1:
					error = error || !checkBgColor(item.Attr, "#CCFFCC")
				case 2:
					error = error || !checkBgColor(item.Attr, "#CCFFCC")
				case 3:
					error = error || !checkBgColor(item.Attr, "#CCFFCC")
				case 4:
					error = error || !checkBgColor(item.Attr, "#CCFFCC")

				}
				count++
			}
			item = item.NextSibling
		}
		if error {
			s := fmt.Sprintf("*FIDO Issue Detected*")
			return errors.New(s)
		}
	}
	return nil

}

func (s *service) sendScreenshot() {
	caps := selenium.Capabilities(map[string]interface{}{"browserName": "chrome"})
	caps["chrome.switches"] = []string{"--ignore-certificate-errors"}
	d, err := selenium.NewRemote(caps, os.Getenv("SELENIUM"))
	if err != nil {
		log.Println(err.Error())
		return
	}

	err = d.Get("http://rc20.sbic.co.za:3073/cics/cwba/tsgweb02")
	if err != nil {
		log.Println(err.Error())
		return
	}

	bytes, err := d.Screenshot()
	if err != nil {
		log.Println(err.Error())
		return
	}

	_, err = http.Post("http://"+os.Getenv("HAL")+"/api/alert/"+os.Getenv("GROUP")+"/image", "text/plain", strings.NewReader(base64.StdEncoding.EncodeToString(bytes)))
	if err != nil {
		log.Println(err.Error())
	}
	d.Close()

}

func (s *service) registerRemoteCommand() {
	for {
		g, err := strconv.ParseInt(os.Getenv("GROUP"), 10, 64)
		if err != nil {
			log.Panic("Environment Valriable GROUP not set to a valid integer")
		}
		request := remoteTelegramCommands.RemoteCommandRequest{Group: g, Name: "fido", Description: "Get a screenshot of the current FIDO state"}
		stream, err := s.client.RegisterCommand(context.Background(), &request)
		if err != nil {
			log.Println(err)
		} else {
			s.monitorForStreamResponse(stream)
		}
		time.Sleep(30 * time.Second)

	}

}

func (s *service) monitorForStreamResponse(client remoteTelegramCommands.RemoteCommand_RegisterCommandClient) {
	for {
		in, err := client.Recv()
		if err != nil {
			log.Println(err)
			return
		}
		log.Println(in.From)
		s.sendScreenshot()
	}
}

func checkBgColor(attrs []html.Attribute, expected string) bool {
	for _, attr := range attrs {
		if attr.Key == "bgcolor" {
			return attr.Val == expected
		}
	}
	return false
}
