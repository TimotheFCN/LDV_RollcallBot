package main

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/apognu/gocal"
	"github.com/joho/godotenv"
	"github.com/madflojo/tasks"
	"github.com/rs/xid"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

var scheduler *tasks.Scheduler
var login string
var password string
var joinApiUrl string
var baseURL = "https://www.leonard-de-vinci.net"
var client http.Client
var lessonList []Lesson

type Lesson struct {
	Description string
	Zoomlink    string
	rollCallURL string
	StartTime   time.Time
	EndTime     time.Time
	isOpen      bool
}

func isAlreadyListed(lesson Lesson) bool {
	for _, l := range lessonList {
		if l.StartTime == lesson.StartTime {
			return true
		}
	}
	return false
}

func main() {
	err := godotenv.Load()
	if err != nil {
		println("Error loading .env file, trying to use ENV variables")
	}

	scheduler = tasks.New()
	defer scheduler.Stop()

	login = os.Getenv("LOGIN")
	password = os.Getenv("PASSWORD")
	joinApiUrl = os.Getenv("JOIN_API_URL")

	jar, err := cookiejar.New(nil)

	client = http.Client{
		Jar: jar,
	}

	authCookies()
	getCalendar()

	//run getcalendar every 12 hours
	ticker := time.NewTicker(time.Hour * 12)
	quit := make(chan struct{})
	go func() {
		for {
			select {
			case <-ticker.C:
				getCalendar()
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()
	<-quit //wait for the goroutine to finish
}

func logError(err error) {
	if err != nil {
		print("[ERROR] ")
		log.Fatal(err)
	}
}

func authCookies() {
	println("[INFO] Getting auth cookies")
	samlLink := func() string {
		//Get SAML Link from ajax
		resp, err := client.PostForm(baseURL+"/ajax.inc.php", url.Values{"act": {"ident_analyse"}, "login": {login}})
		logError(err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		logError(err)
		//Trim the link from the response
		samlLink := string(body[25 : len(body)-3])
		return samlLink
	}
	adfsURL := func() string {
		//Get ADFS URL from SAML Link
		resp, err := client.Get(baseURL + samlLink())
		logError(err)
		defer resp.Body.Close()
		//Get the URL
		adfsURL := resp.Request.URL.String()
		return adfsURL
	}
	samlResponse := func() string {
		//Get the cookies from ADFS by posting the login and password
		resp, err := client.PostForm(adfsURL(), url.Values{"UserName": {login}, "Password": {password}, "AuthMethod": {"FormsAuthentication"}})
		logError(err)
		defer resp.Body.Close()
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		logError(err)
		//Get the SAMLResponse from the form
		samlResponse, _ := doc.Find("input[name=SAMLResponse]").Attr("value")
		return samlResponse
	}
	//Post the SAMLResponse to simpleSAML
	resp, err := client.PostForm(baseURL+"/simplesaml/module.php/saml/sp/saml2-acs.php/alv-sp", url.Values{"SAMLResponse": {samlResponse()}, "RelayState": {samlLink()}})
	logError(err)
	defer resp.Body.Close()
	//Go to the saml link to get the cookies
	resp, err = client.Get(baseURL + samlLink())
	logError(err)
	defer resp.Body.Close()
	//print the body to see if it worked
	_, err = io.ReadAll(resp.Body)
	logError(err)
	println("[INFO] Authenticated")
}

func getCalendar() {
	reAuth()
	calLink := func() string {
		resp, err := client.Get(baseURL)
		logError(err)
		defer resp.Body.Close()
		doc, err := goquery.NewDocumentFromReader(resp.Body)
		logError(err)
		return doc.Find("#main > div:nth-child(3) > div > div > div > header > div > a").AttrOr("href", "error")
	}
	calendar := func() []gocal.Event {
		ical, err := client.Get(calLink())
		logError(err)
		defer ical.Body.Close()
		//Parse the ical
		start, end := time.Now(), time.Now().Add(24*time.Hour)
		c := gocal.NewParser(ical.Body)
		c.Start, c.End = &start, &end
		c.Parse()
		return c.Events
	}

	for _, event := range calendar() {
		lesson := Lesson{
			Description: event.Summary + " à " + event.Start.Format("15:04"),
			Zoomlink:    strings.TrimSpace(event.Description),
			StartTime:   event.Start.Local(),
			EndTime:     event.End.Local(),
		}
		if !isAlreadyListed(lesson) {
			lessonList = append(lessonList, lesson)
			getrollCallURl(lesson) //If available, otherwise it's null
			//checkOpen(lesson)
			go schedule(lesson)
		}
	}
	println("[INFO] Calendar updated")
}

func schedule(lesson Lesson) {
	//Schedule the getrollCall when the lesson starts
	_, err := scheduler.Add(&tasks.Task{
		StartAfter: lesson.StartTime,
		RunOnce:    true,
		TaskFunc: func() error {
			getrollCallURl(lesson)
			return nil
		},
	})
	lesson.rollCallURL = getrollCallURl(lesson)
	//Schedule the checkopen until the lesson ends or the rollCall is opened
	id := xid.New()
	err = scheduler.AddWithID(id.String(), &tasks.Task{
		Interval:   time.Duration(time.Minute * 2),
		StartAfter: lesson.StartTime,
		TaskFunc: func() error {
			if checkOpen(lesson) {
				scheduler.Del(id.String())
				println("[INFO] Task " + lesson.Description + " removed, rollcall opened")
				return nil
			}
			//if the lesson is finished, remove the task
			if time.Now().After(lesson.EndTime) {
				scheduler.Del(id.String())
				println("[INFO] Task " + lesson.Description + " removed, lesson finished")
				return nil
			}
			return nil
		},
	})
	println("[INFO] Task " + lesson.Description + " scheduled")
	logError(err)
}

func getrollCallURl(lesson Lesson) string {
	//Get the rollCall url from the zoom link
	var row []string
	var rows [][]string

	resp, err := client.Get(baseURL + "/student/presences/")
	logError(err)
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	logError(err)
	doc.Find("table").Each(func(index int, tablehtml *goquery.Selection) {
		tablehtml.Find("tr").Each(func(indextr int, rowhtml *goquery.Selection) {
			rowhtml.Find("td").Each(func(indexth int, tablecell *goquery.Selection) {
				links, _ := tablecell.Find("a").Attr("href")
				row = append(row, links)
			})
			rows = append(rows, row)
			row = nil
		})
	})
	for _, row := range rows {
		if len(row) > 5 {
			link := strings.TrimSpace(row[4]) //remove spaces
			if link == lesson.Zoomlink {
				return strings.TrimSpace(row[3])
			}
		}
	}
	return ""
}

func checkOpen(lesson Lesson) bool {
	println("[INFO] Checking if rollCall is open for " + lesson.Description)
	//Check if the rollCall is opened
	resp, err := client.Get(baseURL + lesson.rollCallURL)
	logError(err)
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	logError(err)
	//Check if the rollcall is already validated
	checkValidated := doc.Find("#body_presence > div").Text()
	if strings.Contains(checkValidated, "Vous avez été noté présent") {
		return true
	}
	checkopen := doc.Find("#set-presence").Text()
	if checkopen != "" {
		println("[INFO] rollCall is open for " + lesson.Description)
		lesson.isOpen = true
		sendNotification(lesson)
	}
	return checkopen != ""
}

func sendNotification(lesson Lesson) {
	res, err := client.PostForm(joinApiUrl, url.Values{
		"text":    {lesson.Description},
		"title":   {"Appel ouvert"},
		"icon":    {"https://img.icons8.com/ios-filled/344/attendance-mark.png"},
		"actions": {"Open MyDevinci"},
	})
	logError(err)
	defer res.Body.Close()
}

// Check if the token is still valid
// If not, reauthenticate
func reAuth() {
	resp, err := client.Get(baseURL)
	logError(err)
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	logError(err)
	el := doc.Find("#formWrapper > div.forget-password")
	if el.Nodes != nil || len(el.Nodes) > 0 {
		println("[INFO] Token expired, reauthenticating")
		authCookies()
	}
}

func validate(lesson Lesson) {

}
