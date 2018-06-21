package monitor

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/antchfx/htmlquery"
	"github.com/ernesto-jimenez/httplogger"
	"github.com/kyokomi/emoji"
	"github.com/tebeka/selenium"
	"github.com/weAutomateEverything/go2hal/remoteTelegramCommands"
	"github.com/weAutomateEverything/prognosisHalBot/monitor"
	"golang.org/x/net/html"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func NewService(client remoteTelegramCommands.RemoteCommandClient, store monitor.Store) Service {
	s := service{
		client: client,
		store:  store,
	}

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	http.DefaultClient.Transport = httplogger.NewLoggedTransport(http.DefaultTransport, monitor.NewLogger())
	http.DefaultClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	//Read the Config File
	response, err := http.Get(os.Getenv("CONFIG_FILE"))
	if err != nil {
		panic(err)
	}

	c := config{}
	err = json.NewDecoder(response.Body).Decode(&c)
	if err != nil {
		panic(err)
	}

	s.config = c

	//Register Remote commands
	go func() {
		s.registerStreams()
	}()

	//start monitor
	go func() {
		for true {
			s.runTest()
			time.Sleep(1 * time.Minute)
		}
	}()
	return s
}

type Service interface {
}

type service struct {
	client remoteTelegramCommands.RemoteCommandClient
	store  monitor.Store

	config config
}

type config struct {
	MonitorUrl    string           `json:"monitor_url"`
	ScreenshotUrl string           `json:"screenshot_url"`
	Alerts        map[string]int64 `json:"alerts"`
}

type fidoRow struct {
	name   string
	status string
	dsa    string
	edsa   string
	mxt    string
	error  bool
}

type callout struct {
	Message string `json:"message"`
	Title   string `json:"title"`
}

func (s *service) runTest() {
	t := time.Now()
	//Do not monitor between 00:00 and 06:00 on a Sunday.
	if t.Weekday() == time.Sunday {
		if t.Hour() < 6 {
			return
		}
	}

	resp, err := http.Get(s.config.MonitorUrl)
	if err != nil {
		log.Println(err.Error())
		return
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err.Error())
		return
	}

	doc, err := htmlquery.Parse(bytes.NewReader(b))
	if err != nil {
		log.Println(err.Error())
		return
	}

	nodes := htmlquery.Find(doc, "//tr")
NodeLoop:
	for _, node := range nodes {
		item := node.FirstChild
		count := 0
		issue := false
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
							_, ok := s.config.Alerts[row.name]
							if !ok {
								fmt.Printf("No one is itnerested in %v, ignoring\n", row.name)
								continue NodeLoop
							}
						}
					}

				case 1:
					row.status = item.FirstChild.FirstChild.Data
					issue = issue || !checkBgColor(item.Attr, "#CCFFCC")
				case 2:
					row.dsa = item.FirstChild.FirstChild.Data
					issue = issue || !checkBgColor(item.Attr, "#CCFFCC")
				case 3:
					row.edsa = item.FirstChild.FirstChild.Data
					issue = issue || !checkBgColor(item.Attr, "#CCFFCC")
				case 4:
					row.mxt = item.FirstChild.FirstChild.Data
					issue = issue || !checkBgColor(item.Attr, "#CCFFCC")

				}
				count++
			}
			item = item.NextSibling
		}
		fmt.Printf("Name: %v - status: %v - dsa: %v - edsa %v - max tasks: %v\n", row.name, row.status, row.dsa, row.edsa, row.mxt)
		if issue {
			//Ok - we have found an issue - lets check the config to see if there is a group interested in this error.
			group, ok := s.config.Alerts[row.name]
			if !ok {
				continue NodeLoop
			}

			//K - there is a group - lets send them a screenshot
			s.sendScreenshot(group)

			//Increase the error count
			err := s.store.IncreaseCount(row.name)
			if err != nil {
				log.Println(err.Error())
				continue NodeLoop
			}

			//Get the new error count
			count, err = s.store.GetCount(row.name)
			if err != nil {
				log.Println(err.Error())
				continue NodeLoop
			}

			//If there are 10 errors, we invoke callout
			if count == 10 {
				url := fmt.Sprintf("http://%v/api/callout/%v", os.Getenv("HAL"), group)
				msg := fmt.Sprintf("A fido issue has been detected with region %v.", row.name)
				c := callout{
					Message: msg,
					Title:   msg,
				}
				b, err := json.Marshal(&c)
				if err != nil {
					log.Println(err.Error())
					continue NodeLoop
				}

				bs := bytes.NewReader(b)
				_, err = http.Post(url, "application/json", bs)
				if err != nil {
					log.Println(err.Error())
				}
			}
		} else {
			//No Errors
			group, ok := s.config.Alerts[row.name]
			if !ok {
				continue NodeLoop
			}

			count, err = s.store.GetCount(row.name)
			if count > 0 {
				//Error count is > 0, so there were errors previously - lets tell the group everything is ok now.
				url := fmt.Sprintf("http://%v/api/alert/%v", os.Getenv("HAL"), group)
				msg := emoji.Sprintf(":white_check_mark: %v ok", row.name)
				http.Post(url, "text/plain", strings.NewReader(msg))
				s.store.ZeroCount(row.name)
			}

		}
	}
}

func (s *service) sendScreenshot(group int64) {
	caps := selenium.Capabilities(map[string]interface{}{"browserName": "chrome"})
	caps["chrome.switches"] = []string{"--ignore-certificate-errors"}
	d, err := selenium.NewRemote(caps, os.Getenv("SELENIUM"))
	if err != nil {
		log.Println(err.Error())
		return
	}

	err = d.Get(s.config.ScreenshotUrl)
	if err != nil {
		log.Println(err.Error())
		return
	}

	bytes, err := d.Screenshot()
	if err != nil {
		log.Println(err.Error())
		return
	}

	url := fmt.Sprintf("http://%v/api/alert/%v/image", os.Getenv("HAL"), group)
	_, err = http.Post(url, "text/plain", strings.NewReader(base64.StdEncoding.EncodeToString(bytes)))
	if err != nil {
		log.Println(err.Error())
	}
	d.Close()

}

func (s *service) registerStreams() {
	m := make(map[int64]bool)
	// Find the unique chat id's
	for _, value := range s.config.Alerts {
		_, ok := m[value]
		if !ok {
			m[value] = true
		}
	}

	//loop through the chat id's and setup a remote command

	for key, _ := range m {
		s.registerRemoteCommand(key)
	}

}

func (s *service) registerRemoteCommand(group int64) {
	go func() {
		for {
			request := remoteTelegramCommands.RemoteCommandRequest{Group: group, Name: "fido", Description: "Get a screenshot of the current FIDO state"}
			stream, err := s.client.RegisterCommand(context.Background(), &request)
			if err != nil {
				log.Println(err)
			} else {
				s.monitorForStreamResponse(group, stream)
			}
			time.Sleep(30 * time.Second)

		}
	}()

}

func (s *service) monitorForStreamResponse(group int64, client remoteTelegramCommands.RemoteCommand_RegisterCommandClient) {
	for {
		in, err := client.Recv()
		if err != nil {
			log.Println(err)
			return
		}
		log.Println(in.From)
		s.sendScreenshot(group)
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
